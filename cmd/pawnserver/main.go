package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pawnkit/pawnserver/bundle"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pawnserver:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: pawnserver COMMAND [OPTIONS]")
	}

	if len(args) == 1 && (args[0] == "--version" || args[0] == "-V") {
		fmt.Println(version)

		return nil
	}

	platform := currentPlatform()

	switch args[0] {
	case "pack":
		if len(args) != 3 {
			return errors.New("usage: pawnserver pack DIR BUNDLE")
		}
		if _, err := bundle.VerifyDirectory(args[1], platform); err != nil {
			return err
		}
		return bundle.Pack(args[1], args[2])
	case "verify":
		if len(args) != 2 {
			return errors.New("usage: pawnserver verify PATH")
		}

		_, err := bundle.Verify(args[1], platform)

		return err
	case "inspect":
		if len(args) != 2 {
			return errors.New("usage: pawnserver inspect PATH")
		}

		manifest, err := bundle.Verify(args[1], platform)
		if err != nil {
			return err
		}

		return json.NewEncoder(os.Stdout).Encode(manifest)
	case "install", "update":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		apply := fs.Bool("apply", false, "apply the plan")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 2 {
			return errors.New("usage: pawnserver install [--apply] BUNDLE DIR")
		}
		plan, err := bundle.PlanInstall(fs.Arg(0), fs.Arg(1), platform)
		if err != nil {
			return err
		}
		if err := json.NewEncoder(os.Stdout).Encode(plan); err != nil || !*apply {
			return err
		}
		return bundle.Install(fs.Arg(0), fs.Arg(1), platform)
	case "configure":
		if len(args) != 2 {
			return errors.New("usage: pawnserver configure DIR")
		}

		manifest, err := bundle.VerifyDirectory(args[1], platform)
		if err != nil {
			return err
		}

		if manifest.Configuration == nil {
			return errors.New("bundle does not declare a configuration file")
		}

		return json.NewEncoder(os.Stdout).Encode(manifest.Configuration)
	case "doctor":
		if len(args) != 2 {
			return errors.New("usage: pawnserver doctor DIR")
		}

		manifest, err := bundle.VerifyDirectory(args[1], platform)
		if err != nil {
			return err
		}

		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"name": manifest.Name, "version": manifest.Version, "platform": platform, "verified": true,
		})
	case "run":
		if len(args) != 2 {
			return errors.New("usage: pawnserver run DIR")
		}
		manifest, err := bundle.VerifyDirectory(args[1], platform)
		if err != nil {
			return err
		}
		binary := selectedBinary(manifest, platform)
		command := exec.Command(filepath.Join(args[1], filepath.FromSlash(binary))) //nolint:gosec // Verification selects the declared server binary.
		command.Dir, command.Stdin, command.Stdout, command.Stderr = args[1], os.Stdin, os.Stdout, os.Stderr
		return command.Run()
	case "export-container":
		fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		base := fs.String("base", "scratch", "base container image")
		containerPlatform := fs.String("platform", linuxPlatform(), "bundle platform to export")

		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		if fs.NArg() != 2 {
			return errors.New("usage: pawnserver export-container [--base IMAGE] [--platform PLATFORM] DIR DOCKERFILE")
		}

		if strings.TrimSpace(*base) != *base || strings.ContainsAny(*base, "\r\n\t ") || *base == "" {
			return errors.New("container base image is invalid")
		}

		if !strings.HasPrefix(*containerPlatform, "linux-") || strings.ContainsAny(*containerPlatform, "\r\n\t ") {
			return errors.New("container platform must be a Linux bundle platform")
		}

		manifest, err := bundle.VerifyDirectory(fs.Arg(0), *containerPlatform)
		if err != nil {
			return err
		}

		binary := selectedBinary(manifest, *containerPlatform)
		entrypoint, err := json.Marshal("/server/" + filepath.ToSlash(binary))
		if err != nil {
			return err
		}

		var content strings.Builder
		fmt.Fprintf(&content, "FROM %s\n", *base)
		content.WriteString("COPY . /server\nWORKDIR /server\nENTRYPOINT [")
		content.Write(entrypoint)
		content.WriteString("]\n")

		if manifest.Health != nil && manifest.Health.CheckPort > 0 {
			fmt.Fprintf(&content, "EXPOSE %d\n", manifest.Health.CheckPort)
		}

		return os.WriteFile(fs.Arg(1), []byte(content.String()), 0o644) //nolint:gosec // Container recipes are public files.
	case "rollback":
		if len(args) != 2 {
			return errors.New("usage: pawnserver rollback DIR")
		}
		return bundle.Rollback(args[1])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func currentPlatform() string {
	architecture := runtime.GOARCH
	if architecture == "amd64" {
		architecture = "x86_64"
	}

	return runtime.GOOS + "-" + architecture
}

func linuxPlatform() string {
	platform := currentPlatform()
	_, architecture, _ := strings.Cut(platform, "-")

	return "linux-" + architecture
}

func selectedBinary(manifest bundle.Manifest, platform string) string {
	for _, artifact := range manifest.Server.Binary {
		if artifact.Platform == platform || artifact.Platform == "any" {
			return artifact.Path
		}
	}

	return ""
}
