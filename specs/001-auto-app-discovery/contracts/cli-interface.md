# CLI Interface Specification

**Feature**: 001-auto-app-discovery  
**Version**: 1.0.0

## Commands Overview

| Command | Description | Phase |
|---------|-------------|-------|
| `rad app discover` | Analyze codebase and output discovery.md | Discover |
| `rad app generate` | Generate app.bicep from discovery results | Generate |
| `rad app scaffold` | Run discover + generate in one step | Combined |
| `rad mcp serve` | Start MCP server for AI agent integration | Interface |
| `rad recipe source add` | Configure recipe sources | Configuration |

---

## rad app discover

Analyze a codebase to detect services, dependencies, and team practices.

### Usage

```bash
rad app discover [path] [flags]
```

### Arguments

| Argument | Description | Default |
|----------|-------------|---------|
| `path` | Path to project root | `.` (current directory) |

### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--output` | `-o` | Output file path | `./radius/discovery.md` |
| `--format` | `-f` | Output format (md, json) | `md` |
| `--languages` | `-l` | Languages to analyze (comma-separated) | auto-detect |
| `--min-confidence` | | Minimum confidence threshold | `0.7` |
| `--include-tests` | | Include test services | `false` |
| `--quiet` | `-q` | Suppress progress output | `false` |
| `--verbose` | `-v` | Show detailed analysis info | `false` |

### Examples

```bash
# Analyze current directory
rad app discover

# Analyze specific directory with output path
rad app discover ./my-app -o ./radius/discovery.md

# Analyze only Go and Python code
rad app discover --languages go,python

# JSON output for programmatic use
rad app discover --format json -o discovery.json
```

### Output

Creates `./radius/discovery.md` with:

```markdown
# Application Discovery Report

## Services Detected
- **api-server** (Go) - Port 8080
- **worker** (Python) - Background processor

## Dependencies
| Type | Name | Confidence | Evidence |
|------|------|------------|----------|
| PostgreSQL | main-db | 95% | go.mod: lib/pq |
| Redis | cache | 87% | requirements.txt: redis |

## Team Practices
- Naming: {env}-{service}-{resource}
- Tags: environment, owner, cost-center

## Recommendations
- [ ] Review detected PostgreSQL connection
- [ ] Configure Redis recipe source
```

---

## rad app generate

Generate a Radius application definition from discovery results.

### Usage

```bash
rad app generate [path] [flags]
```

### Arguments

| Argument | Description | Default |
|----------|-------------|---------|
| `path` | Path to discovery.md or project root | `./radius/discovery.md` |

### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--output` | `-o` | Output file path | `./radius/app.bicep` |
| `--discovery` | `-d` | Path to discovery.md | auto-detect |
| `--environment` | `-e` | Target environment name | `default` |
| `--include-recipes` | | Include recipe references | `true` |
| `--dry-run` | | Preview without writing files | `false` |
| `--force` | | Overwrite existing files | `false` |

### Examples

```bash
# Generate from existing discovery
rad app generate

# Generate to specific output
rad app generate -o ./infra/app.bicep

# Preview generation without writing
rad app generate --dry-run

# Target specific environment
rad app generate --environment production
```

### Output

Creates `./radius/app.bicep`:

```bicep
import radius as radius

@description('Radius application for my-app')
resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'my-app'
  properties: {
    environment: environment
  }
}

@description('API Server container')
resource apiServer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'api-server'
  properties: {
    application: app.id
    container: {
      image: 'my-app/api-server:latest'
      ports: {
        http: { containerPort: 8080 }
      }
    }
    connections: {
      postgres: { source: mainDb.id }
    }
  }
}

@description('PostgreSQL database (detected from go.mod)')
resource mainDb 'Applications.Datastores/postgreSqlDatabases@2023-10-01-preview' = {
  name: 'main-db'
  properties: {
    application: app.id
    environment: environment
    recipe: { name: 'postgresql' }
  }
}
```

---

## rad app scaffold

Combined discover + generate workflow.

### Usage

```bash
rad app scaffold [path] [flags]
```

### Flags

Combines flags from both `discover` and `generate` commands.

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--output-dir` | `-o` | Output directory | `./radius` |
| `--environment` | `-e` | Target environment | `default` |
| `--interactive` | `-i` | Prompt for confirmations | `true` |
| `--skip-discovery` | | Use existing discovery.md | `false` |

### Examples

```bash
# Full scaffold with prompts
rad app scaffold ./my-app

# Non-interactive mode
rad app scaffold ./my-app --interactive=false

# Use existing discovery
rad app scaffold --skip-discovery
```

---

## rad mcp serve

Start MCP server for AI agent integration.

### Usage

```bash
rad mcp serve [flags]
```

### Flags

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--transport` | `-t` | Transport type (stdio, http) | `stdio` |
| `--port` | `-p` | HTTP port (when transport=http) | `8765` |
| `--workspace` | `-w` | Default workspace path | `.` |

### Examples

```bash
# Start stdio server (for VS Code)
rad mcp serve

# Start HTTP server
rad mcp serve --transport http --port 8765
```

### MCP Configuration (VS Code)

```json
{
  "mcpServers": {
    "radius": {
      "command": "rad",
      "args": ["mcp", "serve"]
    }
  }
}
```

---

## rad recipe source add

Configure recipe sources for discovery.

### Usage

```bash
rad recipe source add <name> [flags]
```

### Flags

| Flag | Short | Description | Required |
|------|-------|-------------|----------|
| `--type` | `-t` | Source type (avm, terraform, git, local) | Yes |
| `--location` | `-l` | Source location (URL or path) | Yes |
| `--auth` | | Authentication method | No |

### Examples

```bash
# Add Azure Verified Modules
rad recipe source add avm --type avm --location mcr.microsoft.com/bicep/avm

# Add Terraform Registry
rad recipe source add tf-registry --type terraform --location registry.terraform.io

# Add internal Git repository
rad recipe source add internal --type git --location git@github.com:myorg/recipes.git

# Add local directory
rad recipe source add local-dev --type local --location ./recipes
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |
| 3 | Discovery failed |
| 4 | Generation failed |
| 5 | Validation failed |

---

## Environment Variables

| Variable | Description |
|----------|-------------|
| `RADIUS_DISCOVERY_CATALOG` | Path to custom Resource Type catalog |
| `RADIUS_RECIPE_SOURCES` | JSON array of recipe sources |
| `RADIUS_MIN_CONFIDENCE` | Default confidence threshold |
| `RADIUS_OUTPUT_DIR` | Default output directory |
