# Automatic Application Discovery

Radius provides automatic application discovery capabilities to analyze your existing codebase and generate Radius application definitions. This feature helps developers quickly onboard existing applications to Radius.

## Overview

The discovery feature scans your codebase to:

- **Detect infrastructure dependencies** - databases, caches, message queues, and other infrastructure your application uses
- **Identify services** - deployable units based on Dockerfiles, package manifests, and entrypoints
- **Map to Resource Types** - automatically map detected dependencies to Radius Resource Types
- **Apply team practices** - detect and apply naming conventions, tags, and security settings from existing IaC

## Quick Start

### 1. Discover Dependencies

Analyze your codebase to detect infrastructure dependencies:

```bash
rad app discover ./my-project
```

This creates `./radius/discovery.json` and `./radius/discovery.md` with the analysis results.

### 2. Generate Application Definition

Generate a Radius application definition from the discovery results:

```bash
rad app generate
```

This creates `./radius/app.bicep` with your Radius application definition.

### 3. Review and Deploy

Review the generated `app.bicep`, make any necessary customizations, and deploy:

```bash
rad deploy ./radius/app.bicep
```

## Commands

### `rad app discover`

Analyzes a codebase to detect infrastructure dependencies and services.

```bash
rad app discover [path] [flags]
```

**Flags:**
- `-p, --path <path>` - Path to project directory (default: current directory)
- `--min-confidence <float>` - Minimum confidence threshold (0.0-1.0, default: 0.5)
- `--include-dev` - Include development dependencies
- `-o, --output <path>` - Output path for discovery results
- `-y, --accept-defaults` - Accept all defaults without prompting
- `-v, --verbose` - Enable verbose output
- `--dry-run` - Show detected dependencies without writing files

**Example:**
```bash
# Discover in current directory
rad app discover

# Discover in specific path with high confidence threshold
rad app discover ./my-project --min-confidence 0.8

# Dry run to see what would be detected
rad app discover --dry-run --verbose
```

### `rad app generate`

Generates a Radius application definition from discovery results.

```bash
rad app generate [flags]
```

**Flags:**
- `-d, --discovery <path>` - Path to discovery.json (default: ./radius/discovery.json)
- `--app-name <name>` - Application name
- `-e, --environment <name>` - Target Radius environment
- `-o, --output <path>` - Output path for app.bicep (default: ./radius/app.bicep)
- `--comments` - Include helpful comments in generated Bicep (default: true)
- `--recipes` - Include recipe references for infrastructure resources
- `--recipe-profile <profile>` - Recipe profile for environment-specific recipes (e.g., dev, staging, prod)
- `-a, --add-dependency <name>` - Add manual infrastructure dependency (can be repeated)
- `--update` - Update existing app.bicep using diff/patch mode
- `--on-conflict <mode>` - Conflict handling: ask, overwrite, merge, diff, skip
- `-v, --verbose` - Enable verbose output
- `--dry-run` - Show generated Bicep without writing files
- `--validate` - Validate generated Bicep after generation

**Example:**
```bash
# Generate with default settings
rad app generate

# Generate with custom app name and recipes
rad app generate --app-name myapp --recipes

# Add manual dependencies
rad app generate --add-dependency postgres --add-dependency redis

# Generate for production environment
rad app generate --environment prod --recipe-profile prod
```

### `rad app scaffold`

Scaffolds a new Radius application with the necessary files and structure.

```bash
rad app scaffold [flags]
```

**Flags:**
- `-n, --name <name>` - Name of the application (required)
- `-p, --path <path>` - Path where to create the application (default: current directory)
- `-e, --environment <name>` - Target environment name (default: "default")
- `-t, --template <template>` - Application template (e.g., web-api, worker, frontend)
- `-d, --add-dependency <name>` - Add infrastructure dependency (can be repeated)
- `-i, --interactive` - Run in interactive mode (default: true)
- `-f, --force` - Force overwrite if directory exists

**Example:**
```bash
# Scaffold with interactive prompts
rad app scaffold --name myapp

# Scaffold with specific template and dependencies
rad app scaffold --name myapi --template web-api --add-dependency postgres

# Scaffold in specific directory
rad app scaffold --name myapp --path ./projects/myapp
```

### `rad recipe source add`

Adds a recipe source for discovering and matching recipes.

```bash
rad recipe source add [flags]
```

**Flags:**
- `--name <name>` - Name for the recipe source (required)
- `--type <type>` - Source type: avm, terraform, git, local (required)
- `--url <url>` - Source URL or path (required)
- `--priority <int>` - Priority for matching (lower = higher priority)
- `--auth-type <type>` - Authentication type: none, token, basic, credential-helper
- `--auth-token <token>` - Authentication token
- `--auth-token-env <var>` - Environment variable containing auth token
- `--config <path>` - Path to configuration file

**Example:**
```bash
# Add Azure Verified Modules as a source
rad recipe source add --name avm --type avm --url https://registry.terraform.io

# Add internal Terraform registry
rad recipe source add --name internal --type terraform --url https://tf.internal.example.com --auth-type token --auth-token-env TF_TOKEN
```

## MCP Server Integration

For AI coding agents, Radius provides an MCP (Model Context Protocol) server:

```bash
rad mcp serve
```

This exposes all discovery skills as MCP tools that AI agents can invoke. See [MCP Integration](./mcp.md) for details.

## Supported Languages and Frameworks

The discovery feature supports analyzing projects in:

| Language | Package Manifest | Notes |
|----------|-----------------|-------|
| JavaScript/TypeScript | package.json | Detects npm/yarn dependencies |
| Python | requirements.txt, Pipfile, pyproject.toml | Detects pip dependencies |
| Go | go.mod | Detects Go module dependencies |
| Java | pom.xml, build.gradle | Detects Maven/Gradle dependencies |
| C# | .csproj, packages.config | Detects NuGet dependencies |

## Detected Infrastructure Types

The discovery engine can detect and map the following infrastructure types:

| Type | Libraries Detected | Radius Resource Type |
|------|-------------------|---------------------|
| PostgreSQL | pg, psycopg2, lib/pq, npgsql | Applications.Datastores/sqlDatabases |
| MySQL | mysql2, pymysql, go-sql-driver/mysql | Applications.Datastores/sqlDatabases |
| Redis | ioredis, redis, go-redis | Applications.Datastores/redisCaches |
| MongoDB | mongodb, pymongo, mongo-driver | Applications.Datastores/mongoDatabases |
| RabbitMQ | amqplib, pika, streadway/amqp | Applications.Messaging/rabbitMQQueues |
| Kafka | kafkajs, kafka-python, confluent-kafka-go | Applications.Messaging/kafkaQueues |
| Azure Blob Storage | @azure/storage-blob, azure-storage-blob | Various Azure resources |
| AWS S3 | @aws-sdk/client-s3, boto3, aws-sdk-go | Various AWS resources |

## Team Practices

The discovery feature can detect and apply team infrastructure practices from:

- **Configuration files** - `.radius/team-practices.yaml`
- **Terraform files** - `*.tf` files
- **Bicep files** - `*.bicep` files

### Detected Practices

- **Naming conventions** - Resource naming patterns (e.g., `{project}-{env}-{resource}`)
- **Required tags** - Common tags applied to all resources
- **Security settings** - TLS requirements, encryption, private networking
- **Sizing defaults** - Environment-specific SKU/tier settings

### Configuration File

Create `.radius/team-practices.yaml` to explicitly define practices:

```yaml
naming_convention:
  pattern: "{project}-{environment}-{resource}-{instance}"
  separator: "-"
  components:
    - name: project
      required: true
      default_value: myproj
    - name: environment
      required: true
    - name: resource
      required: true
    - name: instance
      default_value: "001"

tags:
  environment: dev
  project: myproject
  owner: platform-team

required_tags:
  - environment
  - project
  - owner

security:
  encryption_enabled: true
  tls_required: true
  min_tls_version: "TLS1_2"
  private_networking: true

sizing:
  default_tier: Standard
  environment_tiers:
    dev:
      tier: Basic
      high_availability: false
    prod:
      tier: Premium
      high_availability: true
      geo_redundant: true
```

## Confidence Levels

Detection results include confidence scores:

- **● High (80%+)** - Strong signal from multiple sources
- **◐ Medium (50-80%)** - Detected but may need verification
- **○ Low (<50%)** - Weak signal, review recommended

Use `--min-confidence` to filter results by confidence threshold.

## Output Files

### discovery.json

Machine-readable JSON containing:
- Detected services
- Infrastructure dependencies
- Resource type mappings
- Confidence scores
- Source evidence

### discovery.md

Human-readable Markdown summary suitable for review and documentation.

### app.bicep

Generated Radius application definition including:
- Application resource
- Container resources for services
- Infrastructure resources for dependencies
- Recipe references (if enabled)

## Best Practices

1. **Review discovery results** - Always review the generated discovery.md before generating the app.bicep
2. **Verify dependencies** - Confirm detected dependencies match your actual usage
3. **Customize app.bicep** - The generated file is a starting point; customize for your needs
4. **Use version control** - Track changes to discovery results and generated files
5. **Configure team practices** - Create `.radius/team-practices.yaml` for consistent conventions
6. **Use recipe profiles** - Separate recipes by environment (dev, staging, prod)

## Troubleshooting

### No dependencies detected

- Verify your project has a supported package manifest
- Check that dependencies are listed in the manifest
- Use `--verbose` to see what files are being analyzed
- Lower the `--min-confidence` threshold

### Incorrect resource type mapping

- Review the confidence score - low confidence may indicate uncertainty
- Manually specify the dependency with `--add-dependency`
- Update the discovery results and regenerate

### app.bicep validation fails

- Ensure the Bicep CLI is installed and up to date
- Check for syntax errors in the generated file
- Review container image references
- Verify environment variable names

## Related Topics

- [Radius Applications](https://docs.radapp.io/concepts/application/)
- [Radius Environments](https://docs.radapp.io/concepts/environment/)
- [Radius Recipes](https://docs.radapp.io/concepts/recipes/)
- [MCP Integration](./mcp.md)
