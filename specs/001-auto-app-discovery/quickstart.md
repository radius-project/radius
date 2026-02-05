# Quickstart: Automatic Application Discovery

Get your existing application running on Radius in minutes.

## Prerequisites

- Radius CLI v0.30+ installed
- An existing codebase with infrastructure dependencies
- (Optional) Kubernetes cluster for deployment

## Step 1: Discover Your Application

Navigate to your project root and run:

```bash
rad app discover
```

This analyzes your codebase and creates `./radius/discovery.md` containing:
- Detected services and entry points
- Infrastructure dependencies (databases, caches, queues)
- Team practices extracted from existing IaC

**Example output:**

```
Analyzing ./my-app...
✓ Detected 2 services: api-server (Go), worker (Python)
✓ Found 3 dependencies: PostgreSQL, Redis, RabbitMQ
✓ Extracted practices from terraform/

Discovery complete → ./radius/discovery.md
```

## Step 2: Review Discoveries

Open `./radius/discovery.md` and review:

1. **Services** - Are all your deployable services detected?
2. **Dependencies** - Check confidence scores and evidence
3. **Recipes** - Matched recipes from your configured sources

Edit the file if needed to add missing items or correct detection errors.

## Step 3: Generate App Definition

```bash
rad app generate
```

This creates `./radius/app.bicep` with:
- Container resources for each service
- Resource types for dependencies
- Recipe references for provisioning
- Connections between services and resources

**Example app.bicep:**

```bicep
import radius as radius

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'my-app'
}

resource apiServer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'api-server'
  properties: {
    application: app.id
    container: {
      image: 'my-app/api-server:latest'
      ports: { http: { containerPort: 8080 } }
    }
    connections: {
      postgres: { source: mainDb.id }
    }
  }
}

resource mainDb 'Radius.Data/postgreSqlDatabases@2025-08-01-preview' = {
  name: 'main-db'
  properties: {
    application: app.id
    recipe: { name: 'kubernetes-postgresql' }
  }
}
```

## Step 4: Deploy

```bash
rad deploy ./radius/app.bicep
```

Radius provisions the infrastructure using recipes and deploys your containers.

---

## One-Step Scaffold

Combine discover and generate:

```bash
rad app scaffold ./my-app
```

This runs both steps and prompts for confirmation at each stage.

---

## Using with AI Agents

### VS Code with GitHub Copilot

1. Configure MCP server in VS Code settings:

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

2. Use natural language:

```
"Analyze my codebase and generate a Radius app"
```

### Programmatic API

```go
import "github.com/radius-project/radius/pkg/discovery"

// Discover dependencies
deps, _ := discovery.DiscoverDependencies(ctx, discovery.Options{
    ProjectPath: "./my-app",
})

// Generate app definition  
app, _ := discovery.GenerateAppDefinition(ctx, discovery.GenerateOptions{
    Dependencies: deps,
    OutputFormat: discovery.FormatBicep,
})
```

---

## Configuring Recipe Sources

Add your organization's recipe repositories:

```bash
# Azure Verified Modules
rad recipe source add avm --type avm --location mcr.microsoft.com/bicep/avm

# Internal repository
rad recipe source add internal --type git --location git@github.com:myorg/recipes.git
```

---

## Supported Languages

| Language | Package Manifest | Entry Point Detection |
|----------|-----------------|----------------------|
| Go | go.mod | main.go |
| Python | requirements.txt, pyproject.toml | main.py, `__main__` |
| JavaScript | package.json | index.js, package.json scripts |
| TypeScript | package.json | index.ts |
| Java | pom.xml, build.gradle | @SpringBootApplication |
| C# | *.csproj | Program.cs |

---

## Detected Dependencies

The discovery engine detects these infrastructure types:

- **Databases**: PostgreSQL, MySQL, MongoDB, SQLite
- **Caches**: Redis, Memcached
- **Message Queues**: RabbitMQ, Kafka, Azure Service Bus
- **Storage**: Azure Blob, AWS S3, MinIO
- **Other**: Elasticsearch, Prometheus

---

## Troubleshooting

### Low confidence scores

If dependencies show low confidence:
1. Check that library imports are in the standard location
2. Ensure package manifest is at project root
3. Add connection string environment variable detection

### Missing services

If services aren't detected:
1. Add a Dockerfile to the service directory
2. Ensure entry point follows language conventions
3. Use `--include-tests` if test services are needed

### Recipe not found

If no recipe matches:
1. Check configured recipe sources: `rad recipe source list`
2. Add internal recipe repository
3. Use a generic recipe and customize
