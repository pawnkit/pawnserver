package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pawnkit/pawnserver/bundle"
)

func TestBundleCLIJourney(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	writeCLIFixture(t, source)
	archive := filepath.Join(dir, "game.pawnbundle")
	destination := filepath.Join(dir, "server")

	for _, args := range [][]string{
		{"verify", source},
		{"pack", source, archive},
		{"install", archive, destination},
		{"install", "--apply", archive, destination},
		{"doctor", destination},
	} {
		if err := run(args); err != nil {
			t.Fatalf("pawnserver %v: %v", args, err)
		}
	}

	if err := os.WriteFile(filepath.Join(destination, "local.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"update", "--apply", archive, destination}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"rollback", destination}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "local.txt")); err != nil {
		t.Fatalf("rollback did not restore the previous installation: %v", err)
	}
}

func TestRunStartsInstalledServer(t *testing.T) {
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "helper")
	if err := os.Mkdir(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	helper := []byte("package main\n\nimport \"os\"\n\nfunc main() { _ = os.WriteFile(\"ran.txt\", []byte(\"ok\"), 0o644) }\n")
	if err := os.WriteFile(filepath.Join(sourceDir, "main.go"), helper, 0o644); err != nil {
		t.Fatal(err)
	}
	binaryName := "server"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	root := filepath.Join(dir, "server")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(root, binaryName)
	command := exec.Command("go", "build", "-o", binaryPath, "main.go")
	command.Dir = sourceDir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("building helper: %v\n%s", err, output)
	}
	if err := os.WriteFile(filepath.Join(root, "main.amx"), []byte("amx"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCLIManifest(t, root, binaryName)

	if err := run([]string{"run", root}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "ran.txt"))
	if err != nil || string(content) != "ok" {
		t.Fatalf("server marker = %q, err = %v", content, err)
	}
}

func writeCLIFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server.bin"), []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.amx"), []byte("amx"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCLIManifest(t, root, "server.bin")
}

func writeCLIManifest(t *testing.T, root, binaryName string) {
	t.Helper()
	binary, err := os.ReadFile(filepath.Join(root, binaryName))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(binary)
	manifest := bundle.Manifest{
		SchemaVersion:  1,
		Name:           "fixture",
		Version:        "1.0.0",
		RuntimeProfile: "openmp",
		Server: bundle.Server{Version: "1.4.0", Binary: []bundle.Artifact{{
			Path: binaryName, Platform: currentPlatform(),
			Checksum: "sha256:" + hex.EncodeToString(sum[:]),
		}}},
		EntryPoints: bundle.EntryPoints{Gamemode: "main.amx"},
	}
	if err := bundle.WriteManifest(filepath.Join(root, bundle.ManifestName), manifest); err != nil {
		t.Fatal(err)
	}
}
