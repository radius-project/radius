#!/bin/bash

set -e

# Set SHELL for pnpm setup (not always set in devcontainer post-create context)
export SHELL="${SHELL:-/bin/bash}"

# Configure pnpm global bin directory
pnpm setup
export PNPM_HOME="$HOME/.local/share/pnpm"
export PATH="$PNPM_HOME:$PATH"

# Configure pnpm store directory inside the container to avoid hard-link issues
# with mounted workspace filesystem (hard links cannot cross filesystem boundaries)
pnpm config set store-dir /tmp/.pnpm-store

# Install TypeSpec first to ensure the language server is available when the VS Code extension loads.
pnpm add -g @typespec/compiler

# Adding workspace as safe directory to avoid permission issues
git config --global --add safe.directory /workspaces/radius

# Install the binary form of golangci-lint, as recommended
# https://golangci-lint.run/welcome/install/#local-installation
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v2.8.0

# Other go tools
go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.0
go install go.uber.org/mock/mockgen@v0.4.0

# Prerequisites for Code Generation, see https://github.com/radius-project/radius/tree/main/docs/contributing/contributing-code/contributing-code-prerequisites#code-generation
cd typespec || exit
pnpm install --force
pnpm add -g autorest@3.7.2 --allow-build=autorest
pnpm add -g oav@4.0.2
