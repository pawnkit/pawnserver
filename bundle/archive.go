package bundle

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	maxExtractedBytes int64 = 2 << 30
	maxArchiveEntries       = 100_000
)

// Pack creates a deterministic bundle archive from root.
func Pack(root, destination string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}

	if pathWithin(root, destination) {
		return errors.New("bundle output cannot be inside its source directory")
	}

	info, err := os.Lstat(root)
	if err != nil {
		return err
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("bundle source must be a directory")
	}

	paths, err := packPaths(root)
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".pawnserver-pack-*")
	if err != nil {
		return err
	}

	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if err := writeArchive(temporary, root, paths); err != nil {
		_ = temporary.Close()

		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	return os.Rename(temporaryPath, destination)
}

func packPaths(root string) ([]string, error) {
	paths := make([]string, 0)
	var total int64

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if path == root {
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not allowed: %s", path)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("unsupported file type: %s", path)
		}

		if info.Mode().IsRegular() {
			total += info.Size()
			if total > maxExtractedBytes {
				return errors.New("bundle exceeds extraction limit")
			}
		}

		paths = append(paths, path)
		if len(paths) > maxArchiveEntries {
			return errors.New("bundle has too many entries")
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)

	return paths, nil
}

func writeArchive(output io.Writer, root string, paths []string) error {
	gzipWriter, err := gzip.NewWriterLevel(output, gzip.BestCompression)
	if err != nil {
		return err
	}

	gzipWriter.ModTime = time.Unix(0, 0)
	gzipWriter.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)

	for _, path := range paths {
		if err := writeArchiveEntry(tarWriter, root, path); err != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()

			return err
		}
	}

	if err := tarWriter.Close(); err != nil {
		_ = gzipWriter.Close()

		return err
	}

	return gzipWriter.Close()
}

func writeArchiveEntry(writer *tar.Writer, root, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
		return fmt.Errorf("unsupported file type: %s", path)
	}

	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}

	header.Name = filepath.ToSlash(relative)
	header.ModTime = time.Unix(0, 0)
	header.AccessTime = time.Time{}
	header.ChangeTime = time.Time{}
	header.Uid = 0
	header.Gid = 0
	header.Uname = ""
	header.Gname = ""
	header.PAXRecords = nil

	if err := writer.WriteHeader(header); err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return nil
	}

	input, err := os.Open(path) //nolint:gosec // The path came from the validated source walk.
	if err != nil {
		return err
	}

	_, copyErr := io.CopyN(writer, input, info.Size())
	closeErr := input.Close()

	return errors.Join(copyErr, closeErr)
}

// Extract unpacks a bundle into destination.
func Extract(archive, destination string) error {
	file, err := os.Open(archive) //nolint:gosec // The caller selects the bundle archive.
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzipReader.Close() }()

	if err := os.MkdirAll(destination, 0o750); err != nil {
		return err
	}

	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	tarReader := tar.NewReader(io.LimitReader(gzipReader, maxExtractedBytes+1))
	var total int64

	entries := 0
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}

		entries++
		if entries > maxArchiveEntries {
			return errors.New("archive has too many entries")
		}

		if !safePath(header.Name) {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}

		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return fmt.Errorf("unsupported archive entry %q", header.Name)
		}

		total += header.Size
		if total > maxExtractedBytes {
			return errors.New("archive exceeds extraction limit")
		}

		name := filepath.FromSlash(normalizeBundlePath(header.Name))
		if header.Typeflag == tar.TypeDir {
			if err := root.MkdirAll(name, 0o750); err != nil {
				return err
			}

			continue
		}

		if err := root.MkdirAll(filepath.Dir(name), 0o750); err != nil {
			return err
		}

		if header.Mode < 0 || header.Mode > 0o777 {
			return fmt.Errorf("invalid archive mode for %q", header.Name)
		}

		mode := os.FileMode(header.Mode) & 0o755
		output, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return err
		}

		_, copyErr := io.CopyN(output, tarReader, header.Size)
		closeErr := output.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return err
		}
	}
}

func pathWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}

	return relative == "." || (relative != ".." && !filepath.IsAbs(relative) && !startsWithParent(relative))
}

func startsWithParent(path string) bool {
	return path == ".." || len(path) > 3 && path[:3] == ".."+string(filepath.Separator)
}
