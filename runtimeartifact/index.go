// Package runtimeartifact selects reviewed server runtime archives.
package runtimeartifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"
)

const maxIndexBytes = 1 << 20

var checksumPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type Index struct {
	SchemaVersion int        `json:"schemaVersion"`
	ID            string     `json:"id"`
	GeneratedAt   string     `json:"generatedAt"`
	Artifacts     []Artifact `json:"artifacts"`
}

type Artifact struct {
	Vendor     string     `json:"vendor"`
	Version    string     `json:"version"`
	Profile    string     `json:"profile"`
	Target     string     `json:"target"`
	Source     Source     `json:"source"`
	Archive    Archive    `json:"archive"`
	Root       string     `json:"root"`
	Executable Executable `json:"executable"`
}

type Source struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Commit     string `json:"commit"`
}

type Archive struct {
	URL      string `json:"url"`
	Format   string `json:"format"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum"`
}

type Executable struct {
	Path         string `json:"path"`
	Architecture string `json:"architecture"`
	Checksum     string `json:"checksum"`
}

// LoadIndex verifies and decodes a runtime index.
func LoadIndex(reader io.Reader, expectedChecksum string) (Index, error) {
	if reader == nil {
		return Index{}, errors.New("runtime index reader is nil")
	}
	if !checksumPattern.MatchString(expectedChecksum) {
		return Index{}, errors.New("runtime index requires a sha256 checksum")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxIndexBytes+1))
	if err != nil {
		return Index{}, fmt.Errorf("read runtime index: %w", err)
	}
	if len(raw) > maxIndexBytes {
		return Index{}, fmt.Errorf("runtime index exceeds %d bytes", maxIndexBytes)
	}
	sum := sha256.Sum256(raw)
	actual := "sha256:" + hex.EncodeToString(sum[:])
	if actual != expectedChecksum {
		return Index{}, fmt.Errorf("runtime index checksum mismatch: got %s, want %s", actual, expectedChecksum)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var index Index
	if err := decoder.Decode(&index); err != nil {
		return Index{}, fmt.Errorf("decode runtime index: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Index{}, errors.New("runtime index contains multiple JSON values")
		}
		return Index{}, fmt.Errorf("decode runtime index: %w", err)
	}
	if err := index.validate(); err != nil {
		return Index{}, err
	}
	return index, nil
}

// Select returns an exact runtime artifact.
func (index Index) Select(vendor, version, target string) (Artifact, error) {
	for _, artifact := range index.Artifacts {
		if artifact.Vendor == vendor && artifact.Version == version && artifact.Target == target {
			return artifact, nil
		}
	}
	return Artifact{}, fmt.Errorf("runtime artifact not found: %s/%s/%s", vendor, version, target)
}

func (index Index) validate() error {
	if index.SchemaVersion != 1 || index.ID == "" || index.GeneratedAt == "" || len(index.Artifacts) == 0 {
		return errors.New("runtime index is missing required fields")
	}
	seen := make(map[string]bool, len(index.Artifacts))
	for _, artifact := range index.Artifacts {
		if err := artifact.validate(); err != nil {
			return err
		}
		key := artifact.Vendor + "\x00" + artifact.Version + "\x00" + artifact.Target
		if seen[key] {
			return fmt.Errorf("duplicate runtime coordinate %s/%s/%s", artifact.Vendor, artifact.Version, artifact.Target)
		}
		seen[key] = true
	}
	return nil
}

func (artifact Artifact) validate() error {
	if artifact.Vendor == "" || artifact.Version == "" || artifact.Profile == "" ||
		artifact.Target == "" || artifact.Archive.Size < 1 {
		return errors.New("runtime artifact is missing required fields")
	}
	if artifact.Archive.Format != "zip" && artifact.Archive.Format != "tar.gz" {
		return fmt.Errorf("unsupported runtime archive format %q", artifact.Archive.Format)
	}
	parsed, err := url.Parse(artifact.Archive.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("invalid runtime archive URL %q", artifact.Archive.URL)
	}
	if !checksumPattern.MatchString(artifact.Archive.Checksum) ||
		!checksumPattern.MatchString(artifact.Executable.Checksum) {
		return errors.New("runtime artifact requires archive and executable checksums")
	}
	if unsafePath(artifact.Root) || unsafePath(artifact.Executable.Path) {
		return errors.New("runtime artifact contains an unsafe path")
	}
	if artifact.Executable.Path != artifact.Root &&
		!strings.HasPrefix(artifact.Executable.Path, artifact.Root+"/") {
		return errors.New("runtime executable is outside the runtime root")
	}
	return nil
}

func unsafePath(value string) bool {
	return value == "" || strings.ContainsRune(value, '\\') || path.IsAbs(value) ||
		path.Clean(value) != value || value == "." || strings.HasPrefix(value, "../")
}
