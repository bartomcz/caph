# caph

```text
 ▄▄▄▄  ▄▄▄  ▄▄▄▄  ▄▄ ▄▄
██▀▀▀ ██▀██ ██▄█▀ ██▄██
▀████ ██▀██ ██    ██ ██
```

`caph` starts a coding harness from a named profile. On Unix systems it replaces
itself with the harness process, so it does not proxy, pipe, or stream terminal
I/O.

## Install

```sh
go install -ldflags "-X main.version=0.1.0 -X main.builtAt=$(date -u +%Y%m%d%H%M%S)" .
```

Alternatively, build a local binary:

```sh
go build -ldflags "-X main.version=0.1.0 -X main.builtAt=$(date -u +%Y%m%d%H%M%S)" -o caph .
```

## Configure

Create `~/.caph/profiles.json`:

```json
{
  "profiles": {
    "foo": {
      "command": "codex",
      "args": ["--model", "o3", "--full-auto"]
    },
    "claude-work": {
      "command": "claude",
      "args": ["--model", "sonnet"],
      "working_directory": "~/projects/work",
      "env": {
        "CLAUDE_CONFIG_DIR": "/path/to/claude/config"
      }
    }
  }
}
```

Each profile supports:

- `command` (required): executable name or path. Its basename must be one of
  `aider`, `amp`, `claude`, `codex`, `gemini`, `goose`, `opencode`, or `pi`.
- `args`: arguments placed before command-line arguments
- `env`: environment variables added to or overriding the current environment
- `working_directory`: directory to enter first; `~` and `~/...` are supported

Unknown configuration fields are rejected to catch mistakes.

## Use

```sh
caph foo
caph foo --additional-harness-argument
caph version
```

To run a profile whose command is not on the supported list, explicitly bypass
the check for that invocation:

```sh
caph --allow-any-command foo
```

Arguments after the profile name are appended after those from the profile.
`caph` resolves the command using `PATH`, changes to the configured working directory if any,
and then uses the operating system's process-replacement operation (`exec`).
Consequently the harness directly owns the existing terminal and receives its
signals.

The command check prevents accidental execution of an unexpected entrypoint; it
is not a sandbox and does not validate arguments or executable contents.

True process replacement is supported on Unix-like operating systems. The
program reports an error on platforms, such as Windows, that do not provide the
same primitive.
