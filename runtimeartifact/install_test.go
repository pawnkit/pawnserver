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
	archive := tarArchive(t, map[string][]byte{
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
	archive := tarArchive(t, map[string][]byte{"Server/omp-server": []byte("server")})
	artifact := testArtifact(archive, []byte("server"))
	artifact.Archive.Checksum = "sha256:" + strings.Repeat("0", 64)
	if err := Install(artifact, bytes.NewReader(archive), filepath.Join(t.TempDir(), "runtime")); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
}

func TestInstallRejectsExecutableChecksumMismatch(t *testing.T) {
	archive := tarArchive(t, map[string][]byte{"Server/omp-server": []byte("server")})
	artifact := testArtifact(archive, []byte("other"))
	if err := Install(artifact, bytes.NewReader(archive), filepath.Join(t.TempDir(), "runtime")); err == nil {
		t.Fatal("executable checksum mismatch accepted")
	}
}

func TestInstallRejectsTarTraversal(t *testing.T) {
	archive := tarArchive(t, map[string][]byte{
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

func tarArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o640, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
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
