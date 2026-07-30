package runtimeartifact

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxArchiveBytes   = 128 << 20
	maxExtractedBytes = 2 << 30
	maxArchiveEntries = 100_000
)

// Install verifies and installs an indexed runtime archive.
func Install(artifact Artifact, archive io.Reader, destination string) error {
	if err := artifact.validate(); err != nil {
		return err
	}
	if archive == nil {
		return errors.New("runtime archive reader is nil")
	}
	if destination == "" {
		return errors.New("runtime destination is empty")
	}

	data, err := io.ReadAll(io.LimitReader(archive, maxArchiveBytes+1))
	if err != nil {
		return fmt.Errorf("read runtime archive: %w", err)
	}
	if len(data) > maxArchiveBytes || int64(len(data)) != artifact.Archive.Size {
		return fmt.Errorf("runtime archive size mismatch: got %d, want %d", len(data), artifact.Archive.Size)
	}
	if actual := contentChecksum(data); actual != artifact.Archive.Checksum {
		return fmt.Errorf("runtime archive checksum mismatch: got %s, want %s", actual, artifact.Archive.Checksum)
	}

	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".pawnserver-runtime-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()

	switch artifact.Archive.Format {
	case "zip":
		err = extractZip(data, staging)
	case "tar.gz":
		err = extractTarGzip(data, staging)
	default:
		err = fmt.Errorf("unsupported runtime archive format %q", artifact.Archive.Format)
	}
	if err != nil {
		return err
	}

	root, err := safeDestination(staging, artifact.Root)
	if err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("runtime root: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("runtime root is not a directory")
	}

	executable, err := safeDestination(staging, artifact.Executable.Path)
	if err != nil {
		return err
	}
	if err := verifyExecutable(executable, artifact.Executable.Checksum); err != nil {
		return err
	}
	if artifact.Target[:strings.IndexByte(artifact.Target, '-')] != "windows" {
		if err := os.Chmod(executable, 0o700); err != nil { //nolint:gosec // The verified server must be executable.
			return fmt.Errorf("mark runtime executable: %w", err)
		}
	}

	return replaceRuntime(root, destination)
}

func extractZip(data []byte, destination string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open runtime archive: %w", err)
	}
	if len(reader.File) > maxArchiveEntries {
		return errors.New("runtime archive has too many entries")
	}
	var total int64
	for _, file := range reader.File {
		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("runtime archive contains a link: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if _, err := safeDestination(destination, file.Name); err != nil {
				return err
			}
			continue
		}
		if file.UncompressedSize64 > uint64(maxExtractedBytes-total) {
			return errors.New("runtime archive exceeds extraction limit")
		}
		size := int64(file.UncompressedSize64) //nolint:gosec // The extraction limit above bounds the conversion.
		total += size
		target, err := safeDestination(destination, file.Name)
		if err != nil {
			return err
		}
		reader, err := file.Open()
		if err != nil {
			return err
		}
		writeErr := writeRegularFile(target, io.LimitReader(reader, size+1))
		closeErr := reader.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return err
		}
	}
	return nil
}

func extractTarGzip(data []byte, destination string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("open runtime archive: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

	reader := tar.NewReader(gzipReader)
	var total int64
	for entries := 0; ; entries++ {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read runtime archive: %w", err)
		}
		if entries >= maxArchiveEntries {
			return errors.New("runtime archive has too many entries")
		}
		target, err := safeDestination(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
		case tar.TypeReg:
			if header.Size < 0 || header.Size > maxExtractedBytes-total {
				return errors.New("runtime archive exceeds extraction limit")
			}
			total += header.Size
			if err := writeRegularFile(target, io.LimitReader(reader, header.Size+1)); err != nil {
				return err
			}
		default:
			return fmt.Errorf("runtime archive contains an unsupported entry: %s", header.Name)
		}
	}
}

func writeRegularFile(target string, reader io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // safeDestination bounds the target.
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	return errors.Join(copyErr, closeErr)
}

func safeDestination(root, name string) (string, error) {
	name = filepath.FromSlash(name)
	if name == "" || filepath.IsAbs(name) || filepath.Clean(name) != name ||
		name == "." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe runtime archive path %q", name)
	}
	target := filepath.Join(root, name)
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe runtime archive path %q", name)
	}
	return target, nil
}

func verifyExecutable(name, expected string) error {
	info, err := os.Lstat(name)
	if err != nil {
		return fmt.Errorf("runtime executable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("runtime executable is not a regular file")
	}
	data, err := os.ReadFile(name) //nolint:gosec // The path is fixed by the verified index.
	if err != nil {
		return err
	}
	if actual := contentChecksum(data); actual != expected {
		return fmt.Errorf("runtime executable checksum mismatch: got %s, want %s", actual, expected)
	}
	return nil
}

func replaceRuntime(stagingRoot, destination string) error {
	backup := destination + ".rollback"
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	hadDestination := false
	if _, err := os.Lstat(destination); err == nil {
		hadDestination = true
		if err := os.Rename(destination, backup); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(stagingRoot, destination); err != nil {
		if hadDestination {
			return errors.Join(err, os.Rename(backup, destination))
		}
		return err
	}
	return nil
}

func contentChecksum(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
