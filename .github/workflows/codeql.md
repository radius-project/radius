# CodeQL Advanced workflow

This workflow performs Static Application Security Testing (SAST) using GitHub CodeQL and GoSec to identify vulnerabilities in the codebase.

## When it runs

- **Pull Requests**: Runs on every PR targeting `main`.
- **Push to Main**: Runs whenever code is merged to `main`.
- **Weekly Schedule**: Runs a full scan every Friday.

## Smart Filtering

To optimize CI feedback time, the workflow applies smart filtering on Pull Requests:

- **Selective Scanning**: Only languages relevant to the changed files are analyzed.
  - Example: If you only change `*.go` files, only Go analysis runs.
  - Example: If you change `.github/workflows`, Actions analysis runs.
- **Full Scans**: Pushes to `main` and scheduled runs always analyze all supported languages (`go`, `javascript`/`typespec`, `actions`).

## Manual Override

If you need to force a full analysis on a Pull Request (ignoring the smart filtering), add the following text to the **PR description** or any **PR comment**:

> `/codeql full`

This is useful when you suspect a change might have side effects in other languages or want to ensure a clean baseline.

## Performance Note

The CodeQL workflow takes longer than standard build workflows because it instruments the build process to create a comprehensive database of the code structure and data flow. This overhead is expected and necessary for deep security analysis.
