package runtimeartifact

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPrepareSessionUsesLocalScriptAndConfiguration(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	template := `{"network":{"port":7777},"pawn":{"main_scripts":["sample 1"],"side_scripts":["extra"]}}`
	if err := os.WriteFile(filepath.Join(runtimeDir, "config.json"), []byte(template), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "project.amx")
	if err := os.WriteFile(script, []byte("amx"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "session")
	plugin := filepath.Join(root, "streamer.so")
	if err := os.WriteFile(plugin, []byte("plugin"), 0o600); err != nil {
		t.Fatal(err)
	}
	disabled := false
	session, err := PrepareSession(runtimeDir, script, destination, SessionOptions{
		Name: "Test server", Port: 7788, Announce: &disabled, EnableQuery: &disabled,
		MaxPlayers: 100, GameMode: "Test mode", RCONPassword: "secret",
		Files:         []SessionFile{{Source: plugin, Destination: "plugins/streamer.so", Checksum: testChecksum("plugin")}},
		LegacyPlugins: []string{"streamer"}, SideScripts: []string{"extra 1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(session.Script); err != nil || string(got) != "amx" {
		t.Fatalf("script = %q, %v", got, err)
	}
	var config map[string]any
	raw, err := os.ReadFile(session.Configuration)
	if err != nil || json.Unmarshal(raw, &config) != nil {
		t.Fatalf("configuration = %q, %v", raw, err)
	}
	network := config["network"].(map[string]any)
	if network["port"] != float64(7788) {
		t.Fatalf("port = %v", network["port"])
	}
	if config["name"] != "Test server" || config["announce"] != false || config["max_players"] != float64(100) {
		t.Fatalf("top-level configuration = %+v", config)
	}
	if config["game"].(map[string]any)["mode"] != "Test mode" {
		t.Fatalf("game configuration = %+v", config["game"])
	}
	if config["rcon"].(map[string]any)["password"] != "secret" {
		t.Fatalf("rcon configuration = %+v", config["rcon"])
	}
	pawn := config["pawn"].(map[string]any)
	if got := pawn["main_scripts"].([]any)[0]; got != "main 1" {
		t.Fatalf("main script = %v", got)
	}
	if got := pawn["legacy_plugins"].([]any)[0]; got != "streamer" {
		t.Fatalf("legacy plugin = %v", got)
	}
	if got := pawn["side_scripts"].([]any)[0]; got != "extra 1" {
		t.Fatalf("side script = %v", got)
	}
	resource, err := os.ReadFile(filepath.Join(destination, "plugins", "streamer.so"))
	if err != nil || string(resource) != "plugin" {
		t.Fatalf("resource = %q, %v", resource, err)
	}
}

func TestPrepareSessionRejectsUnsafeResourceDestination(t *testing.T) {
	root, runtimeDir, script := sessionFixture(t)
	_, err := PrepareSession(runtimeDir, script, filepath.Join(root, "session"), SessionOptions{
		Files: []SessionFile{{Source: script, Destination: "../config.json", Checksum: testChecksum("amx")}},
	})
	if err == nil {
		t.Fatal("unsafe resource destination accepted")
	}
}

func TestPrepareSessionRejectsResourceSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated Windows permissions")
	}
	root, runtimeDir, script := sessionFixture(t)
	link := filepath.Join(root, "plugin.so")
	if err := os.Symlink(script, link); err != nil {
		t.Fatal(err)
	}
	_, err := PrepareSession(runtimeDir, script, filepath.Join(root, "session"), SessionOptions{
		Files: []SessionFile{{Source: link, Destination: "plugins/plugin.so", Checksum: testChecksum("amx")}},
	})
	if err == nil {
		t.Fatal("resource symlink accepted")
	}
}

func TestPrepareSessionRejectsChangedResource(t *testing.T) {
	root, runtimeDir, script := sessionFixture(t)
	plugin := filepath.Join(root, "plugin.so")
	if err := os.WriteFile(plugin, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := PrepareSession(runtimeDir, script, filepath.Join(root, "session"), SessionOptions{
		Files: []SessionFile{{Source: plugin, Destination: "plugins/plugin.so", Checksum: testChecksum("expected")}},
	})
	if err == nil {
		t.Fatal("changed resource accepted")
	}
}

func TestPrepareSessionStagesScriptfiles(t *testing.T) {
	root, runtimeDir, script := sessionFixture(t)
	language := filepath.Join(root, "English")
	if err := os.WriteFile(language, []byte("welcome=Hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "session")
	_, err := PrepareSession(runtimeDir, script, destination, SessionOptions{
		Files: []SessionFile{{
			Source: language, Destination: "scriptfiles/languages/English", Checksum: testChecksum("welcome=Hello"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination, "scriptfiles", "languages", "English"))
	if err != nil || string(got) != "welcome=Hello" {
		t.Fatalf("scriptfile = %q, %v", got, err)
	}
}

func testChecksum(contents string) string {
	sum := sha256.Sum256([]byte(contents))
	return fmt.Sprintf("sha256:%x", sum)
}

func sessionFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "config.json"), []byte(`{"pawn":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "project.amx")
	if err := os.WriteFile(script, []byte("amx"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, runtimeDir, script
}

func TestRunSessionUsesSessionDirectory(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "helper")
	if err := os.Mkdir(source, 0o750); err != nil {
		t.Fatal(err)
	}
	helper := []byte("package main\nimport \"os\"\nfunc main(){_ = os.WriteFile(\"ran.txt\", []byte(\"ok\"), 0600)}\n")
	if err := os.WriteFile(filepath.Join(source, "main.go"), helper, 0o600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "server")
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	command := exec.Command("go", "build", "-o", executable, "main.go")
	command.Dir = source
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, output)
	}
	sessionDir := filepath.Join(root, "session")
	if err := os.Mkdir(sessionDir, 0o750); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(sessionDir, "config.json")
	if err := os.WriteFile(config, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	session := Session{Directory: sessionDir, Configuration: config}
	if err := RunSession(context.Background(), executable, session, ProcessOptions{}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(sessionDir, "ran.txt")); err != nil || string(got) != "ok" {
		t.Fatalf("marker = %q, %v", got, err)
	}
}
