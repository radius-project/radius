#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly GOLANGCI_LINT_VERSION_FILE="${SCRIPT_DIR}/../.golangci-lint-version"

if [[ ! -f "${GOLANGCI_LINT_VERSION_FILE}" ]]; then
    echo "Error: missing golangci-lint version file: ${GOLANGCI_LINT_VERSION_FILE}" >&2
    exit 1
fi

# Strip line endings and surrounding whitespace from the version value.
GOLANGCI_LINT_VERSION="$(tr -d '\r\n' < "${GOLANGCI_LINT_VERSION_FILE}")"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION#"${GOLANGCI_LINT_VERSION%%[![:space:]]*}"}"
GOLANGCI_LINT_VERSION="${GOLANGCI_LINT_VERSION%"${GOLANGCI_LINT_VERSION##*[![:space:]]}"}"

if [[ -z "${GOLANGCI_LINT_VERSION}" ]]; then
    echo "Error: golangci-lint version file is empty: ${GOLANGCI_LINT_VERSION_FILE}" >&2
    exit 1
fi
readonly GOLANGCI_LINT_VERSION

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

# Install the binary form of golangci-lint, as recommended
# https://golangci-lint.run/welcome/install/#local-installation
echo "Installing golangci-lint ${GOLANGCI_LINT_VERSION}..."
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "$(go env GOPATH)/bin" "${GOLANGCI_LINT_VERSION}"

# Other go tools
echo "Installing controller-gen..."
go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.0

echo "Installing mockgen..."
go install go.uber.org/mock/mockgen@v0.4.0

echo "Installing cspell..."
pnpm add -g cspell

echo "============================================================================"
echo "Post-create setup completed successfully!"
echo "============================================================================"
