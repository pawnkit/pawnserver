// Package bundle implements PawnKit server bundles.
package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
)

// ManifestName is the bundle manifest filename.
const ManifestName = "pawn-bundle.json"

// Manifest describes a reproducible server bundle.
type Manifest struct {
	SchemaVersion  int          `json:"schemaVersion"`
	Name           string       `json:"name"`
	Version        string       `json:"version"`
	RuntimeProfile string       `json:"runtimeProfile"`
	Server         Server       `json:"server"`
	EntryPoints    EntryPoints  `json:"entryPoints"`
	Plugins        []Artifact   `json:"plugins,omitempty"`
	Components     []Artifact   `json:"components,omitempty"`
	Configuration  *Config      `json:"configuration,omitempty"`
	Services       []Service    `json:"services,omitempty"`
	Migrations     []Migration  `json:"migrations,omitempty"`
	Persistence    *Persistence `json:"persistence,omitempty"`
	Health         *Health      `json:"health,omitempty"`
	Checksum       string       `json:"checksum"`
}

// Server identifies the runtime and its platform binaries.
type Server struct {
	Version string     `json:"version"`
	Binary  []Artifact `json:"binary"`
}

// EntryPoints lists the scripts loaded by the server.
type EntryPoints struct {
	Gamemode      string   `json:"gamemode"`
	Filterscripts []string `json:"filterscripts,omitempty"`
}

// Artifact is a checksum-verified platform file.
type Artifact struct {
	Name     string `json:"name,omitempty"`
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
	Platform string `json:"platform"`
}

// Config identifies the server configuration file.
type Config struct {
	Path   string `json:"path"`
	Schema string `json:"schema,omitempty"`
}

// Service is an external dependency declared by a bundle.
type Service struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
}

// Migration describes a persistence migration supplied by an operator.
type Migration struct {
	ID               string `json:"id"`
	Description      string `json:"description"`
	AppliesToVersion string `json:"appliesToVersion,omitempty"`
}

// Persistence lists paths kept outside bundle replacement.
type Persistence struct {
	Paths []string `json:"paths,omitempty"`
}

// Health describes an optional runtime health check.
type Health struct {
	CheckCommand string  `json:"checkCommand,omitempty"`
	CheckPort    int     `json:"checkPort,omitempty"`
	Timeout      float64 `json:"timeout,omitempty"`
}

var (
	versionPattern  = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	checksumPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// Load decodes and validates a manifest.
func Load(data []byte) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, err
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("manifest contains trailing data")
	}

	return manifest, manifest.Validate()
}

// Validate checks manifest identity, checksums, and paths.
func (m Manifest) Validate() error {
	if m.SchemaVersion != 1 || m.Name == "" || !versionPattern.MatchString(m.Version) {
		return errors.New("invalid bundle identity")
	}
	if m.RuntimeProfile != "openmp" && m.RuntimeProfile != "samp-037" {
		return errors.New("invalid runtime profile")
	}
	if m.EntryPoints.Gamemode == "" || len(m.Server.Binary) == 0 || m.Server.Version == "" {
		return errors.New("bundle requires a gamemode and server binary")
	}

	paths := []string{m.EntryPoints.Gamemode}
	paths = append(paths, m.EntryPoints.Filterscripts...)

	for _, entry := range paths {
		if !strings.EqualFold(path.Ext(entry), ".amx") {
			return fmt.Errorf("entry point must be an AMX file: %s", entry)
		}
	}

	artifacts := append(append([]Artifact{}, m.Server.Binary...), append(m.Plugins, m.Components...)...)
	artifactChecksums := map[string]string{}

	for _, artifact := range artifacts {
		paths = append(paths, artifact.Path)

		if !checksumPattern.MatchString(artifact.Checksum) {
			return fmt.Errorf("invalid checksum for %s", artifact.Path)
		}

		if artifact.Platform == "" {
			return fmt.Errorf("artifact %s has no platform", artifact.Path)
		}

		normalized := normalizeBundlePath(artifact.Path)
		if checksum, exists := artifactChecksums[normalized]; exists && checksum != artifact.Checksum {
			return fmt.Errorf("conflicting checksums for %s", artifact.Path)
		}

		artifactChecksums[normalized] = artifact.Checksum
	}

	for _, artifact := range append(append([]Artifact{}, m.Plugins...), m.Components...) {
		if artifact.Name == "" {
			return fmt.Errorf("extension %s has no name", artifact.Path)
		}
	}

	if m.Configuration != nil {
		paths = append(paths, m.Configuration.Path)

		const openMPSchema = "https://schemas.pawnkit.dev/openmp-config/v1/schema.json"
		if m.RuntimeProfile == "openmp" && m.Configuration.Schema != openMPSchema {
			return errors.New("open.mp configuration must use the PawnKit open.mp schema")
		}

		if m.RuntimeProfile == "samp-037" && m.Configuration.Schema != "" {
			return errors.New("SA-MP configuration must not declare a JSON schema")
		}
	}

	if m.Persistence != nil {
		paths = append(paths, m.Persistence.Paths...)
	}

	for _, path := range paths {
		if !safePath(path) {
			return fmt.Errorf("unsafe bundle path %q", path)
		}
	}

	if err := validatePersistence(m); err != nil {
		return err
	}

	for _, service := range m.Services {
		if service.Name == "" || service.Kind == "" {
			return errors.New("service name and kind cannot be empty")
		}
	}

	for _, migration := range m.Migrations {
		if migration.ID == "" || migration.Description == "" {
			return errors.New("migration ID and description cannot be empty")
		}
	}

	if m.Health != nil && (m.Health.CheckPort < 0 || m.Health.CheckPort > 65535 || m.Health.Timeout < 0) {
		return errors.New("invalid health check")
	}

	if m.Checksum != "" && !checksumPattern.MatchString(m.Checksum) {
		return errors.New("invalid manifest checksum")
	}
	return nil
}

func validatePersistence(manifest Manifest) error {
	if manifest.Persistence == nil {
		return nil
	}

	managed := []string{manifest.EntryPoints.Gamemode}
	managed = append(managed, manifest.EntryPoints.Filterscripts...)

	for _, artifact := range append(append([]Artifact{}, manifest.Server.Binary...), append(manifest.Plugins, manifest.Components...)...) {
		managed = append(managed, artifact.Path)
	}

	if manifest.Configuration != nil {
		managed = append(managed, manifest.Configuration.Path)
	}

	for _, persistent := range manifest.Persistence.Paths {
		for _, bundled := range managed {
			if pathsOverlap(persistent, bundled) {
				return fmt.Errorf("persistent path %q overlaps bundled file %q", persistent, bundled)
			}
		}
	}

	return nil
}

func pathsOverlap(left, right string) bool {
	left = normalizeBundlePath(left)
	right = normalizeBundlePath(right)

	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

// CanonicalChecksum returns the checksum with Checksum cleared.
func (m Manifest) CanonicalChecksum() (string, error) {
	m.Checksum = ""
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// WriteManifest writes a formatted manifest with its checksum.
func WriteManifest(path string, manifest Manifest) error {
	checksum, err := manifest.CanonicalChecksum()
	if err != nil {
		return err
	}
	manifest.Checksum = checksum
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644) //nolint:gosec // Manifests are public metadata.
}

func safePath(path string) bool {
	if path == "" || strings.Contains(path, "\\") || strings.HasPrefix(path, "/") {
		return false
	}

	for _, char := range path {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}

	clean := normalizeBundlePath(path)

	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func normalizeBundlePath(value string) string { return path.Clean(value) }
