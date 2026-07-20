package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyAcceptsArchive(t *testing.T) {
	archive := testBundleArchive(t, t.TempDir(), "1.0.0", "server\n")

	manifest, err := Verify(archive, "any")
	if err != nil {
		t.Fatal(err)
	}

	if manifest.Name != "fixture" || manifest.Version != "1.0.0" {
		t.Fatalf("manifest = %#v", manifest)
	}
}

func TestVerifyChecksOpenMPEntryPoints(t *testing.T) {
	root := t.TempDir()

	for name, content := range map[string]string{
		"server":                  "server",
		"gamemodes/main.amx":      "main",
		"filterscripts/extra.amx": "extra",
		"config.json":             `{"pawn":{"main_scripts":["wrong"],"side_scripts":["extra"]}}`,
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		mode := os.FileMode(0o644)
		if name == "server" {
			mode = 0o755
		}

		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}

	sum := sha256.Sum256([]byte("server"))
	manifest := Manifest{
		SchemaVersion: 1, Name: "fixture", Version: "1.0.0", RuntimeProfile: "openmp",
		Server: Server{Version: "1.4.0", Binary: []Artifact{{
			Path: "server", Platform: "any", Checksum: "sha256:" + hex.EncodeToString(sum[:]),
		}}},
		EntryPoints:   EntryPoints{Gamemode: "gamemodes/main.amx", Filterscripts: []string{"filterscripts/extra.amx"}},
		Configuration: &Config{Path: "config.json", Schema: "https://schemas.pawnkit.dev/openmp-config/v1/schema.json"},
	}

	if err := WriteManifest(filepath.Join(root, ManifestName), manifest); err != nil {
		t.Fatal(err)
	}

	if _, err := VerifyDirectory(root, "any"); err == nil {
		t.Fatal("mismatched open.mp entry point was accepted")
	}

	config := `{"pawn":{"main_scripts":["main 1"],"side_scripts":["extra"]}}`
	if err := os.WriteFile(filepath.Join(root, "config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := VerifyDirectory(root, "any"); err != nil {
		t.Fatal(err)
	}
}
