---
name: radius-build-cli
description: 'Build the Radius CLI (rad) binary from source. Use when compiling rad, cross-compiling for another platform, creating a debug build, or verifying the CLI builds correctly after code changes.'
argument-hint: 'Optional: build variant (e.g. debug, linux-amd64) — or leave blank for default release build'
---

# Build Radius CLI

Build the `rad` CLI binary from source using the project's Makefile build system.

## Overview

The Radius CLI (`rad`) is a Go binary located at `cmd/rad/main.go`. The build system uses GNU Make with
build configuration split across files in the `build/` directory. The primary build targets are defined
in `build/build.mk` and version information is injected via linker flags defined in `build/version.mk`.

## Build Outputs

Binaries are written to `./dist/<GOOS>_<GOARCH>/<buildtype>/rad` where:
- `<GOOS>` is the target operating system (e.g., `darwin`, `linux`, `windows`)
- `<GOARCH>` is the target architecture (e.g., `amd64`, `arm64`)
- `<buildtype>` is `release` (default) or `debug` (when `DEBUG=1`)

## Procedure

### Step 1: Verify Prerequisites

Confirm the following tools are available before building:

1. **Go**: Run `go version` to verify Go is installed. The required version is specified in `go.mod`.
2. **Make**: Run `make --version` to verify GNU Make is available.
3. **Git**: Run `git rev-parse --is-inside-work-tree` to confirm we are in a Git repository (needed for version injection).

If any prerequisite is missing, stop and report clearly which tool needs to be installed.

### Step 2: Detect Target Platform

Determine the build target platform:

- Run `go env GOOS` to detect the current OS.
- Run `go env GOARCH` to detect the current architecture.

Report the detected platform to the user (e.g., `darwin/arm64`).

### Step 3: Build the CLI

Build the `rad` binary using the Makefile:

```bash
make build-rad
```

This compiles only the `rad` CLI binary for the current OS and architecture. Version metadata
(commit SHA, Git version, release channel, chart version) is injected automatically via `-ldflags`.

**Build variants:**

| Command | Description |
|---|---|
| `make build-rad` | Build `rad` for the current platform (release mode) |
| `DEBUG=1 make build-rad` | Build `rad` with debug symbols (`-gcflags "all=-N -l"`) |
| `make build-rad-<os>-<arch>` | Cross-compile for a specific platform (e.g., `make build-rad-linux-amd64`) |
| `make build` | Build all binaries, packages, and Bicep tooling |

Use `make build-rad` unless the user explicitly requests a different target, debug build, or cross-compilation.

### Step 4: Verify the Build

After the build completes successfully:

1. Confirm the binary exists at the expected output path:
   ```bash
   ls -lh ./dist/$(go env GOOS)_$(go env GOARCH)/release/rad
   ```
   If `DEBUG=1` was used, check `./dist/$(go env GOOS)_$(go env GOARCH)/debug/rad` instead.

2. Run the built binary to verify it executes:
   ```bash
   ./dist/$(go env GOOS)_$(go env GOARCH)/release/rad version
   ```

3. Report the build results:
   - Binary path and size
   - Version information from `rad version` output
   - Build mode (release or debug)

### Step 5: Report Result

Summarize the build:

```
Build complete!
Binary: ./dist/<os>_<arch>/<buildtype>/rad
Version: <version output>
```

## Quick Reference

| Goal | Command |
|------|---------|
| Build rad (current platform) | `make build-rad` |
| Debug build | `DEBUG=1 make build-rad` |
| Cross-compile | `make build-rad-<os>-<arch>` |
| Build everything | `make build` |
