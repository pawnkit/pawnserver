package runtimeartifact

import (
	"context"
	"encoding/json"
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
	disabled := false
	session, err := PrepareSession(runtimeDir, script, destination, SessionOptions{
		Name: "Test server", Port: 7788, Announce: &disabled, EnableQuery: &disabled,
		MaxPlayers: 100, GameMode: "Test mode", RCONPassword: "secret",
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
	if got := len(pawn["side_scripts"].([]any)); got != 0 {
		t.Fatalf("side scripts = %d", got)
	}
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
