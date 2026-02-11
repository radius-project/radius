#!/bin/bash

set -e

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
echo "Installing golangci-lint v2.8.0..."
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v2.8.0

# Other go tools
echo "Installing controller-gen..."
go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.0

echo "Installing mockgen..."
go install go.uber.org/mock/mockgen@v0.4.0

echo "============================================================================"
echo "Post-create setup completed successfully!"
echo "============================================================================"
