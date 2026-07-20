package bundle

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestPackIsDeterministic(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(t.TempDir(), "a.tgz"), filepath.Join(t.TempDir(), "b.tgz")
	if err := Pack(root, a); err != nil {
		t.Fatal(err)
	}
	if err := Pack(root, b); err != nil {
		t.Fatal(err)
	}
	one, _ := os.ReadFile(a)
	two, _ := os.ReadFile(b)
	if string(one) != string(two) {
		t.Fatal("archives differ")
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "bad.tgz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	writer := tar.NewWriter(gz)
	if err := writer.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Extract(archive, t.TempDir()); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestPackRejectsOutputInsideSource(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Pack(root, filepath.Join(root, "bundle.pawnbundle")); err == nil {
		t.Fatal("output inside source was accepted")
	}
}

func TestExtractRejectsEscapingDestinationSymlink(t *testing.T) {
	destination := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(destination, "linked")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	archive := filepath.Join(t.TempDir(), "bad.tgz")
	writeTestArchive(t, archive, "linked/escape", "x")

	if err := Extract(archive, destination); err == nil {
		t.Fatal("escaping destination symlink was accepted")
	}

	if _, err := os.Stat(filepath.Join(outside, "escape")); !os.IsNotExist(err) {
		t.Fatalf("file escaped destination: %v", err)
	}
}

func writeTestArchive(t *testing.T, path, name, content string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)

	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}

	if _, err := tarWriter.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}

	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
