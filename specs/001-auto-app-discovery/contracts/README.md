# API Contracts: Automatic Application Discovery

This directory contains the API contracts for the Discovery feature.

## Contents

- **discovery-api.yaml** - OpenAPI 3.0 spec for Discovery Skills API (programmatic access)
- **mcp-tools.json** - MCP Tool definitions for AI agent integration
- **cli-interface.md** - CLI command specifications

## Usage

### Programmatic API

Import the discovery package and call skills directly:

```go
import "github.com/radius-project/radius/pkg/discovery"

result, err := discovery.DiscoverDependencies(ctx, discovery.Options{
    ProjectPath: "./my-app",
    Languages:   []discovery.Language{discovery.LanguageGo},
})
```

### MCP Tools

Start the MCP server and connect from any MCP-compatible client:

```bash
rad mcp serve
```

### CLI

```bash
rad app discover ./my-app
rad app generate ./my-app
```
