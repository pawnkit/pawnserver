package runtimeartifact

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	maxScriptBytes          = 128 << 20
	maxSessionResourceBytes = 512 << 20
	maxSessionResourceFiles = 4096
)

// SessionFile is a verified project file staged for a server session.
type SessionFile struct {
	Source      string
	Destination string
	Checksum    string
}

type SessionOptions struct {
	Name               string
	Language           string
	Website            string
	Password           string
	Announce           *bool
	EnableQuery        *bool
	MaxPlayers         int
	MaxBots            int
	Sleep              float64
	Port               int
	Bind               string
	LANMode            *bool
	OnFootRate         int
	InVehicleRate      int
	AimingRate         int
	StreamRate         int
	StreamRadius       float64
	PlayerTimeout      int
	AcksLimit          int
	MessagesLimit      int
	MessageHoleLimit   int
	MinConnectionTime  int
	ConnectionSeed     int
	GameMode           string
	MapName            string
	LagCompMode        int
	RCON               *bool
	RCONPassword       string
	LogQueries         *bool
	LogChat            *bool
	LogTimestamps      *bool
	LogTimestampFormat string
	LogDatabase        *bool
	LogDatabaseQueries *bool
	LogCookies         *bool
	Files              []SessionFile
	LegacyPlugins      []string
	SideScripts        []string
}

type Session struct {
	Directory     string
	Configuration string
	Script        string
}

type ProcessOptions struct {
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	Arguments []string
}

// PrepareSession creates an isolated open.mp run directory.
func PrepareSession(runtimeDir, script, destination string, options SessionOptions) (Session, error) {
	if options.Port < 0 || options.Port > 65535 {
		return Session{}, errors.New("runtime port is outside the valid range")
	}
	runtimeDir, err := filepath.Abs(runtimeDir)
	if err != nil {
		return Session{}, err
	}
	script, err = filepath.Abs(script)
	if err != nil {
		return Session{}, err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return Session{}, err
	}
	config, err := readConfiguration(filepath.Join(runtimeDir, "config.json"))
	if err != nil {
		return Session{}, err
	}
	applySessionOptions(config, options)
	pawn := object(config, "pawn")
	pawn["main_scripts"] = []string{"main 1"}
	pawn["legacy_plugins"] = append([]string(nil), options.LegacyPlugins...)
	pawn["side_scripts"] = append([]string(nil), options.SideScripts...)

	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return Session{}, err
	}
	staging, err := os.MkdirTemp(parent, ".pawnserver-session-*")
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	gamemodes := filepath.Join(staging, "gamemodes")
	if err := os.MkdirAll(gamemodes, 0o750); err != nil {
		return Session{}, err
	}
	if err := copyRegularFile(script, filepath.Join(gamemodes, "main.amx"), maxScriptBytes); err != nil {
		return Session{}, err
	}
	if err := stageSessionFiles(staging, options.Files); err != nil {
		return Session{}, err
	}
	configPath := filepath.Join(staging, "config.json")
	if err := writeConfiguration(configPath, config); err != nil {
		return Session{}, err
	}
	if err := replaceRuntime(staging, destination); err != nil {
		return Session{}, err
	}
	return Session{
		Directory:     destination,
		Configuration: filepath.Join(destination, "config.json"),
		Script:        filepath.Join(destination, "gamemodes", "main.amx"),
	}, nil
}

func stageSessionFiles(root string, files []SessionFile) error {
	if len(files) > maxSessionResourceFiles {
		return fmt.Errorf("runtime session exceeds %d resource files", maxSessionResourceFiles)
	}
	seen := make(map[string]bool, len(files))
	var total int64
	for _, file := range files {
		destination, err := sessionDestination(root, file.Destination)
		if err != nil {
			return err
		}
		key := strings.ToLower(filepath.Clean(destination))
		if seen[key] {
			return fmt.Errorf("runtime session destination %q is duplicated", file.Destination)
		}
		seen[key] = true
		info, err := os.Lstat(file.Source)
		if err != nil {
			return fmt.Errorf("runtime session resource %q: %w", file.Destination, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("runtime session resource %q is not a regular file", file.Destination)
		}
		if err := verifySessionFile(file, info.Size()); err != nil {
			return err
		}
		total += info.Size()
		if total > maxSessionResourceBytes {
			return errors.New("runtime session resources exceed size limit")
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
			return err
		}
		if err := copyRegularFile(file.Source, destination, maxSessionResourceBytes); err != nil {
			return fmt.Errorf("runtime session resource %q: %w", file.Destination, err)
		}
	}
	return nil
}

func verifySessionFile(file SessionFile, size int64) error {
	algorithm, encoded, ok := strings.Cut(file.Checksum, ":")
	if !ok || algorithm != "sha256" {
		return fmt.Errorf("runtime session resource %q has an unsupported checksum", file.Destination)
	}
	expected, err := hex.DecodeString(encoded)
	if err != nil || len(expected) != sha256.Size {
		return fmt.Errorf("runtime session resource %q has an invalid checksum", file.Destination)
	}
	source, err := os.Open(file.Source)
	if err != nil {
		return fmt.Errorf("runtime session resource %q: %w", file.Destination, err)
	}
	defer func() { _ = source.Close() }()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(source, size+1))
	if err != nil {
		return fmt.Errorf("runtime session resource %q: %w", file.Destination, err)
	}
	if written != size || subtle.ConstantTimeCompare(hash.Sum(nil), expected) != 1 {
		return fmt.Errorf("runtime session resource %q checksum does not match", file.Destination)
	}
	return nil
}

func sessionDestination(root, name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || filepath.IsAbs(name) {
		return "", fmt.Errorf("runtime session destination %q is unsafe", name)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("runtime session destination %q is unsafe", name)
	}
	prefix, _, _ := strings.Cut(clean, "/")
	switch prefix {
	case "components", "filterscripts", "plugins":
	default:
		return "", fmt.Errorf("runtime session destination %q is not a server resource", name)
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

func applySessionOptions(config map[string]any, options SessionOptions) {
	setString(config, "name", options.Name)
	setString(config, "language", options.Language)
	setString(config, "website", options.Website)
	setString(config, "password", options.Password)
	setBool(config, "announce", options.Announce)
	setBool(config, "enable_query", options.EnableQuery)
	setInt(config, "max_players", options.MaxPlayers)
	setInt(config, "max_bots", options.MaxBots)
	if options.Sleep != 0 {
		config["sleep"] = options.Sleep
	}

	network := object(config, "network")
	setInt(network, "port", options.Port)
	setString(network, "bind", options.Bind)
	setBool(network, "use_lan_mode", options.LANMode)
	setInt(network, "on_foot_sync_rate", options.OnFootRate)
	setInt(network, "in_vehicle_sync_rate", options.InVehicleRate)
	setInt(network, "aiming_sync_rate", options.AimingRate)
	setInt(network, "stream_rate", options.StreamRate)
	setFloat(network, "stream_radius", options.StreamRadius)
	setInt(network, "player_timeout", options.PlayerTimeout)
	setInt(network, "acks_limit", options.AcksLimit)
	setInt(network, "messages_limit", options.MessagesLimit)
	setInt(network, "message_hole_limit", options.MessageHoleLimit)
	setInt(network, "minimum_connection_time", options.MinConnectionTime)
	setInt(network, "cookie_reseed_time", options.ConnectionSeed)

	game := object(config, "game")
	setString(game, "mode", options.GameMode)
	setString(game, "map", options.MapName)
	setInt(game, "lag_compensation_mode", options.LagCompMode)

	rcon := object(config, "rcon")
	setBool(rcon, "enable", options.RCON)
	setString(rcon, "password", options.RCONPassword)

	logging := object(config, "logging")
	setBool(logging, "log_queries", options.LogQueries)
	setBool(logging, "log_chat", options.LogChat)
	setBool(logging, "use_timestamp", options.LogTimestamps)
	setString(logging, "timestamp_format", options.LogTimestampFormat)
	setBool(logging, "log_sqlite", options.LogDatabase)
	setBool(logging, "log_sqlite_queries", options.LogDatabaseQueries)
	setBool(logging, "log_cookies", options.LogCookies)
}

// RunSession starts an installed runtime with an isolated session.
func RunSession(
	ctx context.Context,
	executable string,
	session Session,
	options ProcessOptions,
) error {
	if ctx == nil {
		return errors.New("runtime context is nil")
	}
	info, err := os.Lstat(executable)
	if err != nil {
		return fmt.Errorf("runtime executable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("runtime executable is not a regular file")
	}
	if _, err := os.Stat(session.Configuration); err != nil {
		return fmt.Errorf("runtime configuration: %w", err)
	}
	arguments := []string{"--config-path", session.Configuration}
	arguments = append(arguments, options.Arguments...)
	command := exec.CommandContext(ctx, executable, arguments...) //nolint:gosec // The caller selected a verified runtime.
	command.Dir = session.Directory
	command.Stdin = options.Stdin
	command.Stdout = options.Stdout
	command.Stderr = options.Stderr
	return command.Run()
}

func readConfiguration(path string) (map[string]any, error) {
	file, err := os.Open(path) //nolint:gosec // The runtime directory is caller-selected.
	if err != nil {
		return nil, fmt.Errorf("runtime configuration: %w", err)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxIndexBytes+1))
	if err != nil {
		return nil, fmt.Errorf("runtime configuration: %w", err)
	}
	if len(raw) > maxIndexBytes {
		return nil, errors.New("runtime configuration exceeds size limit")
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("runtime configuration: %w", err)
	}
	if config == nil {
		return nil, errors.New("runtime configuration is empty")
	}
	return config, nil
}

func writeConfiguration(path string, config map[string]any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // The staged path is internal.
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	writeErr := encoder.Encode(config)
	return errors.Join(writeErr, file.Close())
}

func object(parent map[string]any, name string) map[string]any {
	if value, ok := parent[name].(map[string]any); ok {
		return value
	}
	value := make(map[string]any)
	parent[name] = value
	return value
}

func setString(object map[string]any, name, value string) {
	if value != "" {
		object[name] = value
	}
}

func setBool(object map[string]any, name string, value *bool) {
	if value != nil {
		object[name] = *value
	}
}

func setInt(object map[string]any, name string, value int) {
	if value != 0 {
		object[name] = value
	}
}

func setFloat(object map[string]any, name string, value float64) {
	if value != 0 {
		object[name] = value
	}
}

func copyRegularFile(source, destination string, limit int64) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("runtime script is not a regular file")
	}
	if info.Size() > limit {
		return errors.New("runtime script exceeds size limit")
	}
	input, err := os.Open(source) //nolint:gosec // The caller selected the built script.
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // The destination is inside staging.
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, io.LimitReader(input, limit+1))
	return errors.Join(copyErr, output.Close())
}
