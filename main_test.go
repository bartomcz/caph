package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	var stdout bytes.Buffer
	previousVersion, previousBuiltAt := version, builtAt
	version, builtAt = "1.2.3", "20250926123456"
	defer func() { version, builtAt = previousVersion, previousBuiltAt }()

	if err := entrypoint([]string{"version"}, &stdout, &bytes.Buffer{}, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), "caph 1.2.3 (built 20250926123456)\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestLoadConfig(t *testing.T) {
	path := writeTestConfig(t, `{
		"profiles": {
			"foo": {
				"command": "codex",
				"args": ["--model", "o3"],
				"env": {"CODEX_HOME": "/tmp/codex"},
				"working_directory": "/tmp"
			}
		}
	}`)

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	got := cfg.Profiles["foo"]
	if got.Command != "codex" || !reflect.DeepEqual(got.Args, []string{"--model", "o3"}) {
		t.Fatalf("loaded profile = %#v", got)
	}
	if got.Env["CODEX_HOME"] != "/tmp/codex" || got.WorkingDirectory != "/tmp" {
		t.Fatalf("loaded profile = %#v", got)
	}
}

func TestLoadConfigRejectsUnknownField(t *testing.T) {
	path := writeTestConfig(t, `{"profiles":{"foo":{"command":"codex","argz":[]}}}`)
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("loadConfig() error = %v, want unknown field error", err)
	}
}

func TestLoadConfigRequiresCommand(t *testing.T) {
	path := writeTestConfig(t, `{"profiles":{"foo":{"args":["--help"]}}}`)
	_, err := loadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("loadConfig() error = %v, want missing command error", err)
	}
}

func TestMergeEnvironment(t *testing.T) {
	got := mergeEnvironment(
		[]string{"PATH=/bin", "HOME=/home/test"},
		map[string]string{"PATH": "/custom/bin", "TOKEN": "secret"},
	)
	want := []string{"PATH=/custom/bin", "HOME=/home/test", "TOKEN=secret"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeEnvironment() = %#v, want %#v", got, want)
	}
}

func TestLaunchBuildsArgumentsAndEnvironment(t *testing.T) {
	var gotArgv, gotEnv []string
	sentinel := errors.New("replacement returned")
	replacer := func(_ string, argv, env []string) error {
		gotArgv = append([]string(nil), argv...)
		gotEnv = append([]string(nil), env...)
		return sentinel
	}

	err := launch(profile{
		Command: os.Args[0],
		Args:    []string{"configured"},
		Env:     map[string]string{"CAPH_TEST_VALUE": "set"},
	}, []string{"additional"}, true, &bytes.Buffer{}, replacer)
	if err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("launch() error = %v, want wrapped sentinel", err)
	}
	wantArgv := []string{os.Args[0], "configured", "additional"}
	if !reflect.DeepEqual(gotArgv, wantArgv) {
		t.Fatalf("argv = %#v, want %#v", gotArgv, wantArgv)
	}
	if !containsString(gotEnv, "CAPH_TEST_VALUE=set") {
		t.Fatalf("environment does not contain override: %#v", gotEnv)
	}
}

func TestLaunchRejectsUnsupportedCommand(t *testing.T) {
	called := false
	err := launch(profile{Command: "/bin/sh"}, nil, false, &bytes.Buffer{}, func(_ string, _, _ []string) error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), allowAnyCommandFlag) {
		t.Fatalf("launch() error = %v, want explicit bypass error", err)
	}
	if called {
		t.Fatal("process replacer called for unsupported command")
	}
}

func TestSupportedCommand(t *testing.T) {
	for _, command := range []string{"aider", "amp", "claude", "codex", "gemini", "goose", "opencode", "pi", "/usr/local/bin/claude"} {
		if !isSupportedCommand(command) {
			t.Errorf("isSupportedCommand(%q) = false", command)
		}
	}
	if isSupportedCommand("/bin/sh") {
		t.Error("isSupportedCommand(/bin/sh) = true")
	}
}

func TestUnknownProfileErrorSortsNames(t *testing.T) {
	err := unknownProfileError("missing", map[string]profile{"zeta": {}, "alpha": {}})
	if got := err.Error(); !strings.Contains(got, "available: alpha, zeta") {
		t.Fatalf("error = %q", got)
	}
}

func writeTestConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profiles.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
