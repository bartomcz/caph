package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	configFileName      = "profiles.json"
	allowAnyCommandFlag = "--allow-any-command"
)

var version = "dev"
var builtAt = "unknown"

type config struct {
	Profiles map[string]profile `json:"profiles"`
}

type profile struct {
	Description      string            `json:"desc,omitempty"`
	Command          string            `json:"command"`
	Args             []string          `json:"args,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
}

type processReplacer func(path string, argv, env []string) error

func main() {
	if err := entrypoint(os.Args[1:], os.Stdout, os.Stderr, replaceProcess); err != nil {
		fmt.Fprintf(os.Stderr, "caph: %v\n", err)
		os.Exit(1)
	}
}

func entrypoint(args []string, stdout, stderr io.Writer, replace processReplacer) error {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintf(stdout, "caph %s (built %s)\n", version, builtAt)
		return nil
	}

	allowAnyCommand := false
	if len(args) > 0 && args[0] == allowAnyCommandFlag {
		allowAnyCommand = true
		args = args[1:]
	}

	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		printUsage(stderr)
		return nil
	}
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("a command is required")
	}

	command := args[0]
	switch command {
	case "list":
		if len(args) != 1 {
			printUsage(stderr)
			return errors.New("list takes no arguments")
		}
	case "run":
		args = args[1:]
		if len(args) == 0 {
			printUsage(stderr)
			return errors.New("a profile name is required")
		}
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", command)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("find home directory: %w", err)
	}

	path := filepath.Join(home, ".caph", configFileName)
	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}

	if command == "list" {
		for _, name := range profileNames(cfg.Profiles) {
			fmt.Fprintf(stdout, "%s\t%s\n", name, cfg.Profiles[name].Description)
		}
		return nil
	}

	selected, ok := cfg.Profiles[args[0]]
	if !ok {
		return unknownProfileError(args[0], cfg.Profiles)
	}

	return launch(selected, args[1:], allowAnyCommand, stderr, replace)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: caph run <profile> [additional arguments...]")
	fmt.Fprintln(w, "       caph --allow-any-command run <profile> [additional arguments...]")
	fmt.Fprintln(w, "       caph --help")
	fmt.Fprintln(w, "       caph list")
	fmt.Fprintln(w, "       caph version")
}

func printBanner(w io.Writer, harness string) {
	fmt.Fprintf(w, "\x1b[36m ▄▄▄▄  ▄▄▄  ▄▄▄▄  ▄▄ ▄▄\n██▀▀▀ ██▀██ ██▄█▀ ██▄██\n▀████ ██▀██ ██    ██ ██\nPassing to %s...\x1b[0m\n", harness)
}

func loadConfig(path string) (config, error) {
	file, err := os.Open(path)
	if err != nil {
		return config{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer file.Close()

	var cfg config
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	if cfg.Profiles == nil {
		return config{}, fmt.Errorf("parse config %q: missing profiles object", path)
	}
	for name, p := range cfg.Profiles {
		if strings.TrimSpace(name) == "" {
			return config{}, fmt.Errorf("parse config %q: profile name cannot be empty", path)
		}
		if err := validateProfile(name, p); err != nil {
			return config{}, fmt.Errorf("parse config %q: %w", path, err)
		}
	}

	return cfg, nil
}

func validateProfile(name string, p profile) error {
	if strings.TrimSpace(p.Command) == "" {
		return fmt.Errorf("profile %q: command is required", name)
	}
	if strings.IndexByte(p.Command, 0) >= 0 {
		return fmt.Errorf("profile %q: command contains a null byte", name)
	}
	for i, arg := range p.Args {
		if strings.IndexByte(arg, 0) >= 0 {
			return fmt.Errorf("profile %q: argument %d contains a null byte", name, i)
		}
	}
	for key, value := range p.Env {
		if key == "" || strings.ContainsAny(key, "=\x00") {
			return fmt.Errorf("profile %q: invalid environment variable name %q", name, key)
		}
		if strings.IndexByte(value, 0) >= 0 {
			return fmt.Errorf("profile %q: environment variable %q contains a null byte", name, key)
		}
	}
	if strings.IndexByte(p.WorkingDirectory, 0) >= 0 {
		return fmt.Errorf("profile %q: working_directory contains a null byte", name)
	}
	return nil
}

func launch(p profile, additionalArgs []string, allowAnyCommand bool, stderr io.Writer, replace processReplacer) error {
	if !allowAnyCommand && !isSupportedCommand(p.Command) {
		return fmt.Errorf("command %q is not supported; use %s to execute it explicitly", p.Command, allowAnyCommandFlag)
	}

	if p.WorkingDirectory != "" {
		directory, err := expandHome(p.WorkingDirectory)
		if err != nil {
			return err
		}
		if err := os.Chdir(directory); err != nil {
			return fmt.Errorf("change working directory to %q: %w", directory, err)
		}
	}

	path, err := exec.LookPath(p.Command)
	if err != nil {
		return fmt.Errorf("find command %q: %w", p.Command, err)
	}

	argv := make([]string, 0, 1+len(p.Args)+len(additionalArgs))
	argv = append(argv, p.Command)
	argv = append(argv, p.Args...)
	argv = append(argv, additionalArgs...)

	printBanner(stderr, filepath.Base(p.Command))

	if err := replace(path, argv, mergeEnvironment(os.Environ(), p.Env)); err != nil {
		return fmt.Errorf("start command %q: %w", p.Command, err)
	}
	return nil
}

func isSupportedCommand(command string) bool {
	switch filepath.Base(command) {
	case "aider", "amp", "claude", "codex", "gemini", "goose", "opencode", "pi":
		return true
	default:
		return false
	}
}

func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand working directory %q: %w", path, err)
	}
	if path == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
	result := append([]string(nil), base...)
	positions := make(map[string]int, len(base))
	for i, entry := range result {
		if key, _, ok := strings.Cut(entry, "="); ok {
			positions[key] = i
		}
	}

	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry := key + "=" + overrides[key]
		if i, ok := positions[key]; ok {
			result[i] = entry
		} else {
			positions[key] = len(result)
			result = append(result, entry)
		}
	}
	return result
}

func profileNames(profiles map[string]profile) []string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func unknownProfileError(name string, profiles map[string]profile) error {
	names := profileNames(profiles)
	if len(names) == 0 {
		return fmt.Errorf("profile %q not found (no profiles are configured)", name)
	}
	return fmt.Errorf("profile %q not found (available: %s)", name, strings.Join(names, ", "))
}
