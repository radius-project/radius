#!/bin/bash

set -euo pipefail

echo "============================================================================"
echo "Starting post-create setup..."
echo "============================================================================"

# Set SHELL for pnpm setup (not always set in devcontainer post-create context)
echo "Setting SHELL environment variable..."
export SHELL="${SHELL:-/bin/bash}"

# Adding workspace as safe directory to avoid permission issues
echo "Adding workspace as git safe directory..."
git config --global --add safe.directory /workspaces/radius

# Install pnpm via corepack
echo "Installing pnpm via corepack..."
make generate-pnpm-installed

# Configure pnpm store directory inside the container to avoid hard-link issues
# with mounted workspace filesystem (hard links cannot cross filesystem boundaries)
echo "Configuring pnpm store directory..."
pnpm config set store-dir /tmp/.pnpm-store

# Install the binary form of golangci-lint into the Go bin directory (on PATH in
# the dev container). Pinned version + checksums live in build/tools.mk.
echo "Installing golangci-lint..."
GOLANGCI_LINT_INSTALL_DIR="$(go env GOPATH)/bin" make install-golangci-lint

# Install the binary form of shellcheck into the Go bin directory (on PATH in the
# dev container) so 'make lint-shell' works out of the box. Pinned version +
# checksums live in build/tools.mk.
echo "Installing shellcheck..."
SHELLCHECK_INSTALL_DIR="$(go env GOPATH)/bin" make install-shellcheck

echo "Installing cspell..."
# Ensure pnpm global bin directory exists and is on PATH before installing
# global packages. `pnpm setup` updates shell rc files for future sessions,
# but we also need PATH updated for the current script execution.
export PNPM_HOME="${PNPM_HOME:-/home/vscode/.local/share/pnpm}"
PNPM_BIN_DIR="$(pnpm config get global-bin-dir 2>/dev/null || true)"
if [[ -z "${PNPM_BIN_DIR}" || "${PNPM_BIN_DIR}" == "undefined" ]]; then
    PNPM_BIN_DIR="${PNPM_HOME}/bin"
    pnpm config set global-bin-dir "${PNPM_BIN_DIR}"
fi
mkdir -p "${PNPM_BIN_DIR}"
export PATH="${PNPM_BIN_DIR}:${PATH}"
pnpm add -g cspell

echo "============================================================================"
echo "Post-create setup completed successfully!"
echo "============================================================================"
