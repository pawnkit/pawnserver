package bundle

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type InstallPlan struct {
	Destination string `json:"destination"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Replaces    bool   `json:"replaces"`
}

// PlanInstall verifies an archive and describes its destination.
func PlanInstall(archive, destination, platform string) (InstallPlan, error) {
	destination, err := normalizeDestination(destination)
	if err != nil {
		return InstallPlan{}, err
	}

	temp, err := os.MkdirTemp("", "pawnserver-plan-*")
	if err != nil {
		return InstallPlan{}, err
	}
	defer func() { _ = os.RemoveAll(temp) }()
	if err := Extract(archive, temp); err != nil {
		return InstallPlan{}, err
	}
	manifest, err := VerifyDirectory(temp, platform)
	if err != nil {
		return InstallPlan{}, err
	}
	_, statErr := os.Stat(destination)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return InstallPlan{}, statErr
	}

	return InstallPlan{Destination: destination, Name: manifest.Name, Version: manifest.Version, Replaces: statErr == nil}, nil
}

// Install verifies and atomically replaces an installation.
func Install(archive, destination, platform string) error {
	return install(archive, destination, platform, os.Rename)
}

func install(archive, destination, platform string, rename func(string, string) error) error {
	destination, err := normalizeDestination(destination)
	if err != nil {
		return err
	}

	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".pawnserver-stage-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := Extract(archive, staging); err != nil {
		return err
	}
	manifest, err := VerifyDirectory(staging, platform)
	if err != nil {
		return err
	}
	if err := preservePaths(destination, staging, manifest.Persistence); err != nil {
		return err
	}

	if _, err := VerifyDirectory(staging, platform); err != nil {
		return fmt.Errorf("verify staged installation: %w", err)
	}

	backup := destination + ".rollback"
	if err := os.RemoveAll(backup); err != nil {
		return err
	}

	hadDestination := false
	if _, err := os.Stat(destination); err == nil {
		hadDestination = true

		if err := rename(destination, backup); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := rename(staging, destination); err != nil {
		if !hadDestination {
			return fmt.Errorf("install: %w", err)
		}

		if restoreErr := rename(backup, destination); restoreErr != nil {
			return errors.Join(fmt.Errorf("install: %w", err), fmt.Errorf("restore previous installation: %w", restoreErr))
		}
		return fmt.Errorf("install: %w", err)
	}
	return nil
}

func preservePaths(sourceRoot, targetRoot string, persistence *Persistence) error {
	if persistence == nil {
		return nil
	}
	budget := copyBudget{}

	for _, path := range persistence.Paths {
		source := filepath.Join(sourceRoot, filepath.FromSlash(path))
		if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		target := filepath.Join(targetRoot, filepath.FromSlash(path))
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		if err := copyPath(source, target, &budget, 0); err != nil {
			return fmt.Errorf("preserve %s: %w", path, err)
		}
	}
	return nil
}

type copyBudget struct {
	bytes   int64
	entries int
}

func copyPath(source, target string, budget *copyBudget, depth int) error {
	if depth > 128 {
		return errors.New("persistent path is nested too deeply")
	}

	budget.entries++
	if budget.entries > maxArchiveEntries {
		return errors.New("persistent data has too many entries")
	}

	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("symlinks are not allowed")
	}
	if info.Mode().IsRegular() {
		budget.bytes += info.Size()
		if budget.bytes > maxExtractedBytes {
			return errors.New("persistent data exceeds copy limit")
		}

		return copyFile(source, target, info.Mode())
	}
	if !info.IsDir() {
		return errors.New("unsupported file type")
	}
	if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name()), budget, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, target string, mode os.FileMode) error {
	input, err := os.Open(source) //nolint:gosec // The path belongs to verified persistent data.
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm()) //nolint:gosec // The target stays under the staging directory.
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// Rollback restores the previous installation.
func Rollback(destination string) error {
	destination, err := normalizeDestination(destination)
	if err != nil {
		return err
	}

	backup := destination + ".rollback"
	if _, err := os.Stat(backup); err != nil {
		return err
	}
	failed := destination + ".failed"
	if err := os.RemoveAll(failed); err != nil {
		return err
	}

	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, failed); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Rename(backup, destination); err != nil {
		_ = os.Rename(failed, destination)
		return err
	}
	return os.RemoveAll(failed)
}

func normalizeDestination(destination string) (string, error) {
	if destination == "" {
		return "", errors.New("installation destination is empty")
	}

	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}

	absolute = filepath.Clean(absolute)
	if absolute == filepath.VolumeName(absolute)+string(filepath.Separator) {
		return "", errors.New("installation destination cannot be a filesystem root")
	}

	return absolute, nil
}
