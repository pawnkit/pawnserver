package runtimeartifact

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

var coordinatePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// DefaultCacheDir returns the shared PawnKit runtime cache.
func DefaultCacheDir() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "pawnkit", "runtimes"), nil
}

// CacheDestination returns the install directory for an exact runtime.
func CacheDestination(cacheDir, vendor, version, target string) (string, error) {
	if cacheDir == "" {
		return "", errors.New("runtime cache directory is empty")
	}
	for _, value := range []string{vendor, version, target} {
		if !coordinatePattern.MatchString(value) {
			return "", errors.New("runtime coordinate contains an unsafe value")
		}
	}
	return filepath.Join(cacheDir, vendor, version, target), nil
}

// ExecutablePath returns the installed executable path.
func ExecutablePath(artifact Artifact, destination string) (string, error) {
	if err := artifact.validate(); err != nil {
		return "", err
	}
	relative, err := filepath.Rel(filepath.FromSlash(artifact.Root), filepath.FromSlash(artifact.Executable.Path))
	if err != nil {
		return "", err
	}
	return filepath.Join(destination, relative), nil
}
