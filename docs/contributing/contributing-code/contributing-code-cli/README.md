# Develop the Radius CLI

## Purpose

This is the authoritative guide for developing the Radius CLI (`rad`). It covers running `rad` from source, installing a local build, debugging it in VS Code, and the error-handling conventions for CLI code. The CLI entry point is [cmd/rad/main.go](../../../../cmd/rad/main.go); commands live under [cmd/rad/cmd/](../../../../cmd/rad/cmd/) and their implementations under [pkg/cli/](../../../../pkg/cli/). For most CLI work you build `rad` from source instead of using a released binary, so you can test your changes and pick up other contributors' changes. The [first-commit CLI steps](../contributing-code-first-commit/first-commit-03-working-on-cli/index.md) link here for the canonical workflow.

## Prerequisites

- The repository cloned locally and a working build. See [Building the code](../contributing-code-building/README.md).
- Go installed (the version pinned in [go.mod](../../../../go.mod)).
- For debugging: [VS Code](https://code.visualstudio.com/) with the Go extension. The repo ships launch configurations in [.vscode/launch.json](../../../../.vscode/launch.json).

## Steps

### Run rad from source

The fastest way to test a CLI change is `go run`, which builds and runs in one step:

```sh
go run ./cmd/rad/main.go
```

Pass arguments after the path, for example `go run ./cmd/rad/main.go env list`. This must be run from inside the repository so Go can resolve modules.

If you prefer a built binary, run `make build-rad` (see [Building the code](../contributing-code-building/README.md)) and run the binary it writes to `./dist/<GOOS>_<GOARCH>/release/rad`.

### Create a wrapper script (optional)

If you frequently run a local build of `rad`, wrap `go run` in a script so it behaves like the real command. Create a file named `dev-rad` on your `PATH`:

```sh
#!/bin/sh
set -eu
go run /path/to/radius/cmd/rad/main.go "$@"
```

Replace `/path/to/radius` with your repository root, then `chmod +x dev-rad`. Because it uses `go run`, the wrapper only works when your shell's working directory is inside the repository.

### Install a local build

Use the Makefile to install a local build. By default it installs to `/usr/local/bin/rad`, which may require `sudo` depending on the destination's permissions:

```sh
sudo make install
```

Override the destination with `RAD_LOCATION`. The path must end in `rad` and should be on your `PATH`:

```sh
RAD_LOCATION=/my/custom/location/rad sudo make install
```

### Debug rad in VS Code

The repo's [.vscode/launch.json](../../../../.vscode/launch.json) defines the **"Debug rad CLI (prompt for args)"** configuration, which launches [cmd/rad/main.go](../../../../cmd/rad/main.go) and prompts you for the command-line arguments to run.

1. Set a breakpoint — click the gutter to the left of the line numbers. For example, break at the start of the `Run` method in [pkg/cli/cmd/version/version.go](../../../../pkg/cli/cmd/version/version.go) to debug `rad version`.
2. Open the **Run and Debug** pane and select **"Debug rad CLI (prompt for args)"** from the drop-down.
3. Click the green triangle to start. The project builds first (this can take a moment).
4. When prompted, enter the arguments to debug (for example `version`, or `env list`) and confirm. Execution stops at your breakpoint.

> ⚠️ VS Code debugging does not support interactive user input. To debug a command that prompts for confirmation, pass `--yes` in the arguments to bypass the prompt. This does not apply to `rad init`, which is always interactive.
>
> 📝 On **macOS**, the first debug session with a new Go version may take 1–2 minutes and prompt for your password — this is expected.

### Write code for the CLI

Classify errors as *expected* or *unexpected*:

- **Expected** — a known state that is not a bug, such as "application not found". Return these as plain user-facing messages using `clierrors.Message` or `clierrors.MessageWithCause`. Write complete sentences ending with a period, for example `"The application could not be found."`.
- **Unexpected** — an unknown state that could be a Radius bug, such as a partial HTTP response. Return these as-is so the user sees full troubleshooting information, optionally wrapping them with context.

```go
// Classify expected errors and return them as basic user-facing messages.
result, err := findApplication(id)
if errors.Is(err, NotFoundError{}) {
    return clierrors.Message("The application %q could not be found.", applicationName)
} else if err != nil {
    // optional: wrap the error to add context.
    return fmt.Errorf("error retrieving application: %w", err)
}
```

## Verification

- `go run ./cmd/rad/main.go version` prints version information that includes your local changes.
- After `sudo make install`, running `rad version` from any directory resolves to your freshly installed build.
- A breakpoint set in `cmd/rad/` is hit when you run **"Debug rad CLI (prompt for args)"**.

## Troubleshooting

- **`go run` fails to resolve modules.** Run it from inside the repository; Go needs the module context.
- **The debugger never stops at your breakpoint.** Confirm you selected **"Debug rad CLI (prompt for args)"** and that the breakpoint is on an executable line in code the command actually reaches.
- **A debugged command hangs waiting for input.** Add `--yes` to the prompted arguments (except for `rad init`).
- **`make install` is denied.** The destination needs elevated permissions; prefix with `sudo` or point `RAD_LOCATION` at a writable directory on your `PATH`.
