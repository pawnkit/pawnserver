package runtimeartifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallTarGzipRuntime(t *testing.T) {
	executable := []byte("server")
	archive := tarArchive(t, []tarEntry{
		{name: "Server/", directory: true},
		{name: "Server/omp-server", content: executable},
		{name: "Server/config.json", content: []byte("{}")},
	})
	artifact := testArtifact(archive, executable)
	destination := filepath.Join(t.TempDir(), "runtime")

	if err := Install(artifact, bytes.NewReader(archive), destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination, "config.json"))
	if err != nil || string(got) != "{}" {
		t.Fatalf("config = %q, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(destination, "omp-server"))
	if err != nil || info.Mode()&0o100 == 0 {
		t.Fatalf("executable mode = %v, %v", info, err)
	}
}

func TestInstallRejectsBackslashTraversal(t *testing.T) {
	executable := []byte("server")
	archive := tarArchive(t, []tarEntry{
		{name: `..\outside`, content: []byte("bad")},
		{name: "Server/omp-server", content: executable},
	})
	artifact := testArtifact(archive, executable)
	if err := Install(artifact, bytes.NewReader(archive), filepath.Join(t.TempDir(), "runtime")); err == nil {
		t.Fatal("backslash traversal accepted")
	}
}

func TestInstallTarGzipRuntimeWithoutDirectoryEntries(t *testing.T) {
	executable := []byte("server")
	archive := tarArchiveFiles(t, map[string][]byte{
		"Server/omp-server":  executable,
		"Server/config.json": []byte("{}"),
	})
	artifact := testArtifact(archive, executable)
	destination := filepath.Join(t.TempDir(), "runtime")

	if err := Install(artifact, bytes.NewReader(archive), destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination, "config.json"))
	if err != nil || string(got) != "{}" {
		t.Fatalf("config = %q, %v", got, err)
	}
	info, err := os.Stat(filepath.Join(destination, "omp-server"))
	if err != nil || info.Mode()&0o100 == 0 {
		t.Fatalf("executable mode = %v, %v", info, err)
	}
}

func TestInstallRejectsArchiveChecksumMismatch(t *testing.T) {
	archive := tarArchiveFiles(t, map[string][]byte{"Server/omp-server": []byte("server")})
	artifact := testArtifact(archive, []byte("server"))
	artifact.Archive.Checksum = "sha256:" + strings.Repeat("0", 64)
	if err := Install(artifact, bytes.NewReader(archive), filepath.Join(t.TempDir(), "runtime")); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
}

func TestInstallRejectsExecutableChecksumMismatch(t *testing.T) {
	archive := tarArchiveFiles(t, map[string][]byte{"Server/omp-server": []byte("server")})
	artifact := testArtifact(archive, []byte("other"))
	if err := Install(artifact, bytes.NewReader(archive), filepath.Join(t.TempDir(), "runtime")); err == nil {
		t.Fatal("executable checksum mismatch accepted")
	}
}

func TestInstallRejectsTarTraversal(t *testing.T) {
	archive := tarArchiveFiles(t, map[string][]byte{
		"../outside":        []byte("bad"),
		"Server/omp-server": []byte("server"),
	})
	artifact := testArtifact(archive, []byte("server"))
	if err := Install(artifact, bytes.NewReader(archive), filepath.Join(t.TempDir(), "runtime")); err == nil {
		t.Fatal("archive traversal accepted")
	}
}

func testArtifact(archive, executable []byte) Artifact {
	return Artifact{
		Vendor:  "openmultiplayer",
		Version: "1",
		Profile: "openmp",
		Target:  "linux-amd64",
		Archive: Archive{
			URL:      "https://example.test/runtime.tar.gz",
			Format:   "tar.gz",
			Size:     int64(len(archive)),
			Checksum: contentChecksum(archive),
		},
		Root: "Server",
		Executable: Executable{
			Path:         "Server/omp-server",
			Architecture: "386",
			Checksum:     contentChecksum(executable),
		},
	}
}

type tarEntry struct {
	name      string
	content   []byte
	directory bool
}

func tarArchiveFiles(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	entries := make([]tarEntry, 0, len(files))
	for name, content := range files {
		entries = append(entries, tarEntry{name: name, content: content})
	}
	return tarArchive(t, entries)
}

func tarArchive(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Mode: 0o640, Size: int64(len(entry.content))}
		if entry.directory {
			header.Typeflag = tar.TypeDir
			header.Size = 0
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(entry.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
