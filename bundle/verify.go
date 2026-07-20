package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const maxMetadataBytes int64 = 8 << 20

// Verify checks a bundle directory or archive.
func Verify(path, platform string) (Manifest, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Manifest{}, err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return Manifest{}, errors.New("bundle path cannot be a symlink")
	}

	if info.IsDir() {
		return VerifyDirectory(path, platform)
	}

	if !info.Mode().IsRegular() {
		return Manifest{}, errors.New("bundle must be a directory or regular archive")
	}

	temporary, err := os.MkdirTemp("", "pawnserver-verify-*")
	if err != nil {
		return Manifest{}, err
	}
	defer func() { _ = os.RemoveAll(temporary) }()

	if err := Extract(path, temporary); err != nil {
		return Manifest{}, err
	}

	return VerifyDirectory(temporary, platform)
}

// VerifyDirectory checks a bundle directory for the selected platform.
func VerifyDirectory(root, platform string) (Manifest, error) {
	data, _, err := readRegularFile(root, ManifestName, maxMetadataBytes)
	if err != nil {
		return Manifest{}, err
	}

	manifest, err := Load(data)
	if err != nil {
		return Manifest{}, err
	}

	want, err := manifest.CanonicalChecksum()
	if err != nil {
		return Manifest{}, err
	}

	if manifest.Checksum != want {
		return Manifest{}, errors.New("manifest checksum mismatch")
	}

	matched := false
	for _, artifact := range manifest.Server.Binary {
		if artifact.Platform == platform || artifact.Platform == "any" {
			matched = true
		}
	}

	artifacts := append(append([]Artifact{}, manifest.Server.Binary...), append(manifest.Plugins, manifest.Components...)...)
	for _, artifact := range artifacts {
		if artifact.Platform != platform && artifact.Platform != "any" {
			continue
		}

		if err := verifyArtifact(root, artifact, executablePlatform(platform) && slices.Contains(manifest.Server.Binary, artifact)); err != nil {
			return Manifest{}, err
		}
	}

	if !matched {
		return Manifest{}, fmt.Errorf("bundle has no server binary for %s", platform)
	}

	paths := append([]string{manifest.EntryPoints.Gamemode}, manifest.EntryPoints.Filterscripts...)
	if manifest.Configuration != nil {
		paths = append(paths, manifest.Configuration.Path)
	}

	for _, path := range paths {
		if _, err := statRegularFile(root, path); err != nil {
			return Manifest{}, fmt.Errorf("required file %s: %w", path, err)
		}
	}

	if err := verifyConfiguration(root, manifest); err != nil {
		return Manifest{}, err
	}

	return manifest, nil
}

func verifyArtifact(root string, artifact Artifact, executable bool) error {
	file, info, err := openRegularFile(root, artifact.Path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if executable && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("server binary is not executable: %s", artifact.Path)
	}

	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maxExtractedBytes+1))
	if err != nil {
		return err
	}

	if written > maxExtractedBytes {
		return fmt.Errorf("artifact exceeds verification limit: %s", artifact.Path)
	}

	checksum := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if artifact.Checksum != checksum {
		return fmt.Errorf("checksum mismatch: %s", artifact.Path)
	}

	return nil
}

func readRegularFile(root, name string, limit int64) ([]byte, os.FileInfo, error) {
	file, info, err := openRegularFile(root, name)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, nil, err
	}

	if int64(len(data)) > limit {
		return nil, nil, fmt.Errorf("file exceeds verification limit: %s", name)
	}

	return data, info, nil
}

func statRegularFile(root, name string) (os.FileInfo, error) {
	path := filepath.Join(root, filepath.FromSlash(name))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}

	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("file is not regular")
	}

	return info, nil
}

func openRegularFile(root, name string) (*os.File, os.FileInfo, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rootHandle.Close() }()

	name = filepath.FromSlash(name)
	info, err := rootHandle.Lstat(name)
	if err != nil {
		return nil, nil, err
	}

	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("file is not regular")
	}

	file, err := rootHandle.Open(name)
	if err != nil {
		return nil, nil, err
	}

	return file, info, nil
}

func verifyConfiguration(root string, manifest Manifest) error {
	if manifest.RuntimeProfile != "openmp" || manifest.Configuration == nil {
		return nil
	}

	data, _, err := readRegularFile(root, manifest.Configuration.Path, maxMetadataBytes)
	if err != nil {
		return err
	}

	var configuration struct {
		Pawn struct {
			MainScripts []string `json:"main_scripts"`
			SideScripts []string `json:"side_scripts"`
		} `json:"pawn"`
	}

	if err := json.Unmarshal(data, &configuration); err != nil {
		return fmt.Errorf("read open.mp configuration: %w", err)
	}

	mainScripts := normalizeScriptNames(configuration.Pawn.MainScripts)
	wantMain := []string{normalizeScriptName(manifest.EntryPoints.Gamemode)}
	if !slices.Equal(mainScripts, wantMain) {
		return fmt.Errorf("open.mp main scripts do not match bundle entry point")
	}

	sideScripts := normalizeScriptNames(configuration.Pawn.SideScripts)
	wantSide := normalizeScriptNames(manifest.EntryPoints.Filterscripts)
	slices.Sort(sideScripts)
	slices.Sort(wantSide)

	if !slices.Equal(sideScripts, wantSide) {
		return errors.New("open.mp side scripts do not match bundle entry points")
	}

	return nil
}

func normalizeScriptNames(values []string) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = normalizeScriptName(value)
	}

	return result
}

func normalizeScriptName(value string) string {
	if fields := strings.Fields(value); len(fields) > 0 {
		value = fields[0]
	}

	value = filepath.ToSlash(value)
	value = strings.TrimSuffix(value, filepath.Ext(value))

	return filepath.Base(value)
}

func executablePlatform(platform string) bool {
	return strings.HasPrefix(platform, "linux-") || strings.HasPrefix(platform, "darwin-")
}
