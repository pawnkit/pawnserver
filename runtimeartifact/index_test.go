package runtimeartifact

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestLoadIndexAndSelect(t *testing.T) {
	document := indexDocument("Server", "Server/omp-server")
	index, err := LoadIndex(strings.NewReader(document), checksum(document))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := index.Select("openmultiplayer", "1.5.8.3079", "linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Profile != "openmp" || artifact.Executable.Architecture != "386" {
		t.Fatalf("artifact = %+v", artifact)
	}
}

func TestLoadIndexRejectsChecksumMismatch(t *testing.T) {
	_, err := LoadIndex(strings.NewReader(indexDocument("Server", "Server/omp-server")),
		"sha256:"+strings.Repeat("0", 64))
	if err == nil {
		t.Fatal("checksum mismatch accepted")
	}
}

func TestLoadIndexRejectsUnsafeRoot(t *testing.T) {
	document := indexDocument("../Server", "Server/omp-server")
	if _, err := LoadIndex(strings.NewReader(document), checksum(document)); err == nil {
		t.Fatal("unsafe root accepted")
	}
}

func TestLoadIndexRejectsExecutableOutsideRoot(t *testing.T) {
	document := indexDocument("Server", "other/omp-server")
	if _, err := LoadIndex(strings.NewReader(document), checksum(document)); err == nil {
		t.Fatal("external executable accepted")
	}
}

func indexDocument(root, executable string) string {
	return `{
		"schemaVersion":1,
		"id":"test",
		"generatedAt":"2026-07-30T00:00:00Z",
		"artifacts":[{
			"vendor":"openmultiplayer",
			"version":"1.5.8.3079",
			"profile":"openmp",
			"target":"linux-amd64",
			"source":{"repository":"openmultiplayer/open.mp","tag":"v1","commit":"c6759bd8d265171ae3d86598895a23d5a8d92a3b"},
			"archive":{"url":"https://github.com/openmultiplayer/open.mp/releases/download/v1/server.tar.gz","format":"tar.gz","size":10,"checksum":"sha256:` +
		strings.Repeat("1", 64) + `"},
			"root":"` + root + `",
			"executable":{"path":"` + executable + `","architecture":"386","checksum":"sha256:` +
		strings.Repeat("2", 64) + `"}
		}]
	}`
}

func checksum(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
