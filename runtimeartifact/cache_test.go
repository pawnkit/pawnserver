package runtimeartifact

import (
	"path/filepath"
	"testing"
)

func TestCacheDestination(t *testing.T) {
	got, err := CacheDestination("cache", "openmultiplayer", "1.5.8.3079", "linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("cache", "openmultiplayer", "1.5.8.3079", "linux-amd64")
	if got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}
}

func TestCacheDestinationRejectsTraversal(t *testing.T) {
	if _, err := CacheDestination("cache", "..", "1", "linux-amd64"); err == nil {
		t.Fatal("unsafe coordinate accepted")
	}
}

func TestExecutablePath(t *testing.T) {
	artifact := testArtifact([]byte("archive"), []byte("server"))
	got, err := ExecutablePath(artifact, filepath.Join("cache", "runtime"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("cache", "runtime", "omp-server")
	if got != want {
		t.Fatalf("executable = %q, want %q", got, want)
	}
}
