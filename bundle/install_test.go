package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallUpdateAndRollback(t *testing.T) {
	dir := t.TempDir()
	first := testBundleArchive(t, dir, "1.0.0", "first\n")
	destination := filepath.Join(dir, "server")

	plan, err := PlanInstall(first, destination, "any")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Replaces || plan.Version != "1.0.0" {
		t.Fatalf("plan = %+v", plan)
	}
	if err := Install(first, destination, "any"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(destination, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "data", "state.db"), []byte("saved"), 0o644); err != nil {
		t.Fatal(err)
	}

	second := testBundleArchive(t, dir, "1.1.0", "second\n")
	plan, err = PlanInstall(second, destination, "any")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Replaces || plan.Version != "1.1.0" {
		t.Fatalf("plan = %+v", plan)
	}
	if err := Install(second, destination, "any"); err != nil {
		t.Fatal(err)
	}
	if got := testRead(t, filepath.Join(destination, "server")); got != "second\n" {
		t.Fatalf("server = %q", got)
	}
	if got := testRead(t, filepath.Join(destination, "data", "state.db")); got != "saved" {
		t.Fatalf("state = %q", got)
	}
	if err := Rollback(destination); err != nil {
		t.Fatal(err)
	}
	if got := testRead(t, filepath.Join(destination, "server")); got != "first\n" {
		t.Fatalf("server = %q", got)
	}
}

func TestInstallRestoresDestinationWhenReplacementFails(t *testing.T) {
	dir := t.TempDir()
	first := testBundleArchive(t, dir, "1.0.0", "first\n")
	second := testBundleArchive(t, dir, "1.1.0", "second\n")
	destination := filepath.Join(dir, "server")
	if err := Install(first, destination, "any"); err != nil {
		t.Fatal(err)
	}

	replacementFailed := false
	rename := func(oldPath, newPath string) error {
		if newPath == destination && !replacementFailed {
			replacementFailed = true
			return errors.New("injected replacement failure")
		}
		return os.Rename(oldPath, newPath)
	}
	if err := install(second, destination, "any", rename); err == nil {
		t.Fatal("replacement succeeded")
	}
	if got := testRead(t, filepath.Join(destination, "server")); got != "first\n" {
		t.Fatalf("server = %q", got)
	}
}

func TestInstallPreservesBackupWhenRestorationFails(t *testing.T) {
	dir := t.TempDir()
	first := testBundleArchive(t, dir, "1.0.0", "first\n")
	second := testBundleArchive(t, dir, "1.1.0", "second\n")
	destination := filepath.Join(dir, "server")
	if err := Install(first, destination, "any"); err != nil {
		t.Fatal(err)
	}

	rename := func(oldPath, newPath string) error {
		if newPath == destination {
			return errors.New("injected rename failure")
		}
		return os.Rename(oldPath, newPath)
	}
	err := install(second, destination, "any", rename)
	if err == nil || !strings.Contains(err.Error(), "install:") || !strings.Contains(err.Error(), "restore previous installation:") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination+".rollback", "server")); err != nil {
		t.Fatalf("rollback copy was not preserved: %v", err)
	}
}

func TestInstallWithoutPreviousDestinationReportsRenameFailure(t *testing.T) {
	dir := t.TempDir()
	archive := testBundleArchive(t, dir, "1.0.0", "first\n")
	destination := filepath.Join(dir, "server")

	rename := func(_, _ string) error { return errors.New("injected rename failure") }
	err := install(archive, destination, "any", rename)

	if err == nil || !strings.Contains(err.Error(), "install:") || strings.Contains(err.Error(), "restore previous installation") {
		t.Fatalf("error = %v", err)
	}
}

func TestInstallRejectsFilesystemRoot(t *testing.T) {
	archive := testBundleArchive(t, t.TempDir(), "1.0.0", "first\n")
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)

	if err := Install(archive, root, "any"); err == nil {
		t.Fatal("filesystem root was accepted")
	}
}

func TestInstallRejectsPersistentSymlink(t *testing.T) {
	dir := t.TempDir()
	first := testBundleArchive(t, dir, "1.0.0", "first\n")
	second := testBundleArchive(t, dir, "1.1.0", "second\n")
	destination := filepath.Join(dir, "server")
	if err := Install(first, destination, "any"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(destination, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("outside", filepath.Join(destination, "data", "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := Install(second, destination, "any"); err == nil || !strings.Contains(err.Error(), "symlinks are not allowed") {
		t.Fatalf("error = %v", err)
	}
	if got := testRead(t, filepath.Join(destination, "server")); got != "first\n" {
		t.Fatalf("server = %q", got)
	}
}

func testBundleArchive(t *testing.T, parent, version, server string) string {
	t.Helper()
	root := filepath.Join(parent, "bundle-"+version)
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server"), []byte(server), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.amx"), []byte("amx"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(server))
	manifest := Manifest{
		SchemaVersion:  1,
		Name:           "fixture",
		Version:        version,
		RuntimeProfile: "openmp",
		Server: Server{Version: "1.4.0", Binary: []Artifact{{
			Path: "server", Platform: "any", Checksum: "sha256:" + hex.EncodeToString(sum[:]),
		}}},
		EntryPoints: EntryPoints{Gamemode: "main.amx"},
		Persistence: &Persistence{Paths: []string{"data"}},
	}
	if err := WriteManifest(filepath.Join(root, ManifestName), manifest); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(parent, "bundle-"+version+".pawnbundle")
	if err := Pack(root, archive); err != nil {
		t.Fatal(err)
	}
	return archive
}

func testRead(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
