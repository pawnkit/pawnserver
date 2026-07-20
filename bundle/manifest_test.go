package bundle_test

import (
	"testing"

	"github.com/pawnkit/pawnserver/bundle"
)

func TestManifestRejectsEscapingPaths(t *testing.T) {
	m := bundle.Manifest{SchemaVersion: 1, Name: "x", Version: "1.0.0", RuntimeProfile: "openmp", Server: bundle.Server{Binary: []bundle.Artifact{{Path: "../server", Platform: "any", Checksum: "sha256:" + zeros(64)}}}, EntryPoints: bundle.EntryPoints{Gamemode: "main.amx"}}
	if err := m.Validate(); err == nil {
		t.Fatal("unsafe path accepted")
	}
}

func TestManifestRejectsRootPersistencePath(t *testing.T) {
	m := bundle.Manifest{SchemaVersion: 1, Name: "x", Version: "1.0.0", RuntimeProfile: "openmp", Server: bundle.Server{Binary: []bundle.Artifact{{Path: "server", Platform: "any", Checksum: "sha256:" + zeros(64)}}}, EntryPoints: bundle.EntryPoints{Gamemode: "main.amx"}, Persistence: &bundle.Persistence{Paths: []string{"."}}}
	if err := m.Validate(); err == nil {
		t.Fatal("root persistence path accepted")
	}
}

func TestManifestAcceptsServicesAndMigrations(t *testing.T) {
	manifest := validManifest()
	manifest.Services = []bundle.Service{{Name: "mysql", Kind: "database", Required: true}}
	manifest.Migrations = []bundle.Migration{{ID: "users-v2", Description: "add user index", AppliesToVersion: "1.1.0"}}

	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestManifestRejectsPersistentManagedFile(t *testing.T) {
	manifest := validManifest()
	manifest.Persistence = &bundle.Persistence{Paths: []string{"gamemodes"}}

	if err := manifest.Validate(); err == nil {
		t.Fatal("persistent path overlapping the gamemode was accepted")
	}
}

func TestManifestRejectsTrailingJSON(t *testing.T) {
	if _, err := bundle.Load([]byte(`{} {}`)); err == nil {
		t.Fatal("trailing JSON was accepted")
	}
}

func validManifest() bundle.Manifest {
	return bundle.Manifest{
		SchemaVersion:  1,
		Name:           "fixture",
		Version:        "1.0.0",
		RuntimeProfile: "openmp",
		Server: bundle.Server{Version: "1.4.0", Binary: []bundle.Artifact{{
			Path: "server", Platform: "any", Checksum: "sha256:" + zeros(64),
		}}},
		EntryPoints: bundle.EntryPoints{Gamemode: "gamemodes/main.amx"},
	}
}

func zeros(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '0'
	}
	return string(b)
}
