#!/bin/bash

set -e

# Adding workspace as safe directory to avoid permission issues
git config --global --add safe.directory /workspaces/radius 

# Install the binary form of golangci-lint, as recommended
# https://golangci-lint.run/welcome/install/#local-installation
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v1.64.6

# Other go tools
go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.0 
go install go.uber.org/mock/mockgen@v0.4.0

# Prerequisites for Code Generation, see https://github.com/radius-project/radius/tree/main/docs/contributing/contributing-code/contributing-code-prerequisites#code-generation
cd typespec || exit 
npm ci 
npm install -g autorest 
npm install -g oav 
