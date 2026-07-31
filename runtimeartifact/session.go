package runtimeartifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

const maxScriptBytes = 128 << 20

type SessionOptions struct {
	Port int
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
	if options.Port != 0 {
		network := object(config, "network")
		network["port"] = options.Port
	}
	pawn := object(config, "pawn")
	pawn["main_scripts"] = []string{"gamemodes/main 1"}
	pawn["side_scripts"] = []string{}

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
