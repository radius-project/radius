# Feature Specification: Automatic Application Discovery

**Feature Branch**: `001-auto-app-discovery`  
**Created**: January 28, 2026  
**Status**: Draft 

## Vision

Radius will make it trivial for developers to adopt Radius for existing and new applications by removing the need to manually define Resource Types and Recipes.

**How it works:**

1. **Understand the codebase** — Detect infrastructure dependencies (e.g., PostgreSQL, Redis) from library usage in the code.
2. **Match to proven IaC** — Find Recipes from internal Terraform/Bicep repositories or external authoritative sources (Azure Verified Modules)
3. **Apply team practices** — Detect naming conventions, tags, and cost practices from existing IaC files, config, or internal documentation.
4. **Generate the application** — Produce a complete, editable Radius application definition (Bicep) ready to deploy.

**The result**: Developers go from codebase to deployable Radius application with zero manual Resource Type or Recipe authoring.

## End-to-End User Journey *(mandatory)*

Radius Discovery is designed **skills-first**. All capabilities are implemented as composable skills that power multiple interfaces: AI coding agents (via MCP), the `rad` CLI, and a programmatic API. Whether you're chatting with Copilot or typing commands, you're invoking the same underlying engine.

### The Scenario

Throughout this journey, we follow a **Node.js e-commerce application** with:
- **Services**: `api-server` (Express.js on port 3000), `worker` (background processor)
- **Dependencies**: PostgreSQL database, Redis cache, Azure Blob Storage

The goal: Go from existing codebase → deployable Radius application with zero manual Resource Type or Recipe authoring.

---

### The Workflow

```
     USER                                RADIUS                           EXTERNAL SOURCES
       |                                   |                                     |
       |  1. DISCOVER                      |                                     |
       |  "Analyze my codebase"            |                                     |
       |---------------------------------->|                                     |
       |                                   |  Analyze codebase...                |
       |                                   |  - Detect dependencies              |
       |                                   |  - Find services                    |
       |                                   |  - Extract team practices           |
       |                                   |                                     |
       |  ./radius/discovery.md            |                                     |
       |<----------------------------------|                                     |
       |                                   |                                     |
       |  2. GENERATE                      |                                     |
       |  "Create my app definition"       |                                     |
       |---------------------------------->|                                     |
       |                                   |  Generate Resource Types...         |
       |                                   |  (applying team practices)          |
       |                                   |                                     |
       |                                   |  Search for Recipes...              |
       |                                   |------------------------------------>|
       |                                   |                 Internal Terraform, |
       |                                   |<-------------- Bicep repos, AVM     |
       |                                   |                                     |
       |  Recipe Options:                  |                                     |
       |  PostgreSQL -> [1] Azure (AVM)    |                                     |
       |               [2] Container       |                                     |
       |<----------------------------------|                                     |
       |                                   |                                     |
       |  Select: 1                        |                                     |
       |---------------------------------->|                                     |
       |                                   |  Generate app.bicep...              |
       |                                   |  Validate...                        |
       |                                   |                                     |
       |  ./radius/app.bicep               |                                     |
       |<----------------------------------|                                     |
       |                                   |                                     |
       |  3. DEPLOY                        |                                     |
       |  "Deploy to production"           |                                     |
       |---------------------------------->|                                     |
       |                                   |  Provision infrastructure...        |
       |                                   |------------------------------------>|
       |                                   |                    Azure PostgreSQL |
       |                                   |<-------------------Azure Redis      |
       |                                   |                    Storage Account  |
       |                                   |  Deploy containers...               |
       |                                   |                                     |
       |  Deployment Complete!             |                                     |
       |  https://my-app.azurecontainer... |                                     |
       |<----------------------------------|                                     |
       v                                   v                                     v
```

---

### Skills-First Architecture

All interfaces (AI agents, CLI, API) invoke the same underlying skills:

```
+--------------------------------------------------------------------------+
|                          RADIUS DISCOVERY                                |
+--------------------------------------------------------------------------+
|                                                                          |
|    +------------------+   +------------------+   +------------------+    |
|    |    AI Agents     |   |     rad CLI      |   |  Programmatic    |    |
|    |   (via MCP)      |   |                  |   |      API         |    |
|    +--------+---------+   +--------+---------+   +--------+---------+    |
|             |                      |                      |              |
|             +----------------------+----------------------+              |
|                                    |                                     |
|                                    v                                     |
|    +----------------------------------------------------------------+    |
|    |                        SKILLS LAYER                            |    |
|    +----------------------------------------------------------------+    |
|    |  discover_dependencies | discover_services | discover_team_    |    |
|    |  discover_recipes | generate_resource_types | generate_app_    |    |
|    |  validate_app_definition                                       |    |
|    +----------------------------------------------------------------+    |
|                                    |                                     |
|                                    v                                     |
|    +----------------------------------------------------------------+    |
|    |                        CORE ENGINE                             |    |
|    +----------------------------------------------------------------+    |
|    |  Language Analyzers | Team Practices Analyzer | Bicep Generator|    |
|    +----------------------------------------------------------------+    |
|                                                                          |
+--------------------------------------------------------------------------+
```

**Skills Reference:**

| Skill | Phase | Description | Output |
|-------|-------|-------------|--------|
| `discover_dependencies` | 1 | Analyze codebase for infrastructure dependencies | `{dependencies: [{type, technology, confidence, evidence}]}` |
| `discover_services` | 1 | Find deployable services/entrypoints | `{services: [{name, type, port, entrypoint}]}` |
| `discover_team_practices` | 1 | Detect conventions from IaC, config, documentation | `{practices: [{category, convention, source, environment}]}` |
| `generate_resource_types` | 2 | Create Resource Type schemas with team conventions | `{resourceTypes: [{name, schema, outputs}]}` |
| `discover_recipes` | 2 | Find matching IaC implementations from sources | `{recipes: [{resourceType, name, source, iacType}]}` |
| `generate_app_definition` | 2 | Assemble into deployable app.bicep | `{path: string, content: string}` |
| `validate_app_definition` | 2 | Validate generated Bicep syntax and references | `{valid: boolean, errors: []}` |

---

### Phase 1: Discover (`rad app discover .`)

<table>
<tr>
<td width="50%"><strong>Via AI Agent</strong></td>
<td width="50%"><strong>Via CLI</strong></td>
</tr>
<tr>
<td>

```
User: "Help me deploy my Node.js 
       e-commerce app using Radius"

Agent: I'll analyze your codebase.
       [Invokes: discover_dependencies]
       [Invokes: discover_services]
       [Invokes: discover_team_practices]
       
       📦 Services detected:
       • api-server (Express.js, :3000)
       • worker (background processor)
       
       🔌 Dependencies found:
       • PostgreSQL (95%) - pg package
       • Redis (92%) - ioredis package  
       • Azure Blob (88%) - @azure/storage-blob
       
       📋 Team practices detected:
       • Naming: {env}-{service}-{resource}
       • Tags: cost-center, owner, environment
       • Dev: Basic tier, no HA
       • Prod: Premium tier, HA, geo-redundant
       (Sources: /infra/*.tf, team wiki)
       
       📄 Output: ./radius/discovery.md

       Ready to generate your app definition?
```

</td>
<td>

```bash
$ rad app discover .

🔍 Analyzing codebase...

✅ Discovery Complete

Detected Dependencies:
┌────────────────┬────────────┬────────────┐
│ Technology     │ Confidence │ Evidence   │
├────────────────┼────────────┼────────────┤
│ PostgreSQL     │ 95%        │ pg@8.11.0  │
│ Redis          │ 92%        │ ioredis@5.3│
│ Azure Blob     │ 88%        │ @azure/blob│
└────────────────┴────────────┴────────────┘

Services Detected:
• api-server (Express.js, port 3000)
• worker (background job processor)

Team Practices Detected:
• Naming: {env}-{service}-{resource}
• Required Tags: cost-center, owner, env
• Dev: Basic tier, auto-shutdown 7PM
• Prod: Premium, HA enabled, geo-redundant
  (Sources: Terraform in /infra, team wiki)

📄 Output: ./radius/discovery.md

Run 'rad app generate' to create your 
application definition.
```

</td>
</tr>
</table>

---

### Phase 2: Generate (`rad app generate`)

<table>
<tr>
<td width="50%"><strong>Via AI Agent</strong></td>
<td width="50%"><strong>Via CLI</strong></td>
</tr>
<tr>
<td>

```
User: "Yes, use Azure managed services 
       for production"

Agent: [Invokes: generate_resource_types]
       [Invokes: discover_recipes(
         profile: "production")]
       
       I'll create Resource Types applying 
       your team's practices:
       
       • PostgreSQL → HA enabled, 4 vCores
         (per team convention)
       • Redis → Premium tier
       • Blob → Geo-redundant storage
       
       Found matching IaC implementations:
       • PostgreSQL → Azure Database for 
         PostgreSQL Flexible Server (AVM)
       • Redis → Azure Cache for Redis (AVM)
       • Blob → Azure Storage Account (AVM)
       
       Generating your application...
       
       [Invokes: generate_app_definition]
       [Invokes: validate_app_definition]
       
       ✅ Created ./radius/app.bicep
       
       Your application definition includes:
       • 2 containers (api-server, worker)
       • 3 Resource Types with schemas
       • 3 Recipes (Azure Verified Modules)
       • Connection strings auto-wired
       
       Deploy with: rad deploy -e prod
```

</td>
<td>

```bash
$ rad app generate

📋 Generating Resource Types...
   ✓ Applications.Datastores/postgreSql
   ✓ Applications.Datastores/redis  
   ✓ Applications.Datastores/blobStorage

🔍 Discovering IaC implementations...

PostgreSQL:
  [1] Azure Database for PostgreSQL (AVM)
      ✓ Production  ✓ HA  ✓ Auto-backup
  [2] PostgreSQL Container (Dev)
      ⚡ Fast  ⚠ Not for production

Select Recipe for PostgreSQL [1]: 1

Redis:
  [1] Azure Cache for Redis (AVM)
  [2] Redis Container (Dev)

Select Recipe for Redis [1]: 1

Azure Blob Storage:
  [1] Azure Storage Account (AVM)

Select Recipe [1]: 1

📝 Generating application definition...
   ✓ Wiring container connections
   ✓ Mapping environment variables
   ✓ Validating Bicep syntax

✅ Created: ./radius/app.bicep

Application Structure:
┌─────────────────────────────────┐
│       my-ecommerce-app          │
│              │                  │
│    ┌─────────┼─────────┐        │
│    ▼         ▼         ▼        │
│ api-server  worker   (infra)    │
│   :3000                         │
│    └─────────┬─────────┘        │
│              │                  │
│    ┌─────────┼─────────┐        │
│    ▼         ▼         ▼        │
│ Postgres   Redis    Blob        │
│  (AVM)     (AVM)   Storage      │
└─────────────────────────────────┘
```

</td>
</tr>
</table>

**Output**: `./radius/app.bicep` — a fully editable Radius application definition containing:
- Container definitions for detected services
- Resource Type definitions (interfaces) for each dependency
- Recipe references (IaC implementations) for provisioning infrastructure
- Connection wiring between containers and resources

---

### Phase 3: Deploy (`rad deploy ./radius/app.bicep -e <env>`)

<table>
<tr>
<td width="50%"><strong>Via AI Agent</strong></td>
<td width="50%"><strong>Via CLI</strong></td>
</tr>
<tr>
<td>

```
User: "Deploy it to my dev environment"

Agent: I'll deploy to your dev environment.
       
       $ rad deploy ./radius/app.bicep -e dev
       
       ✓ Provisioning PostgreSQL... [2m 34s]
       ✓ Provisioning Redis... [1m 12s]
       ✓ Provisioning Blob Storage... [45s]
       ✓ Deploying api-server... [38s]
       ✓ Deploying worker... [32s]
       
       ✅ Deployment Complete!
       
       Endpoint: https://my-ecommerce-app-
                 api.azurecontainerapps.io
```

</td>
<td>

```bash
$ rad deploy ./radius/app.bicep -e dev

🚀 Deploying my-ecommerce-app...

✓ Creating Resource Types (if needed)
✓ Provisioning product-db... [2m 34s]
✓ Provisioning session-cache... [1m 12s]
✓ Provisioning image-storage... [45s]
✓ Deploying api-server... [38s]
✓ Deploying worker... [32s]

✅ Deployment Complete!

Application Endpoints:
• api-server: https://my-ecommerce-app-
              api.azurecontainerapps.io

Run 'rad app status' for health info.
```

</td>
</tr>
</table>

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Analyze Existing Application Codebase (Priority: P1)

As a developer with an existing application, I want Radius to analyze my codebase and automatically detect the infrastructure dependencies my application requires (such as databases, message queues, caches, and storage), so that I can adopt Radius without manually defining Resource Types.

**Why this priority**: This is the foundational capability that enables zero-friction adoption. Without automatic detection of dependencies, developers must still manually configure Radius, which defeats the purpose of simplifying adoption.

**Independent Test**: Can be fully tested by pointing Radius at a sample codebase with known dependencies (e.g., a Node.js app with PostgreSQL and Redis libraries) and verifying it correctly identifies those dependencies.

**Acceptance Scenarios**:

1. **Given** a codebase with a PostgreSQL client library (e.g., `pg` for Node.js, `psycopg2` for Python), **When** I run the discovery command, **Then** Radius identifies a PostgreSQL database dependency.
2. **Given** a codebase with multiple infrastructure dependencies, **When** I run the discovery command, **Then** Radius produces a dependency report listing all detected dependencies with confidence scores.
3. **Given** a codebase with no detectable infrastructure dependencies, **When** I run the discovery command, **Then** Radius informs the user that no dependencies were found and suggests manual configuration options.

---

### User Story 2 - Map to Resource Types from Detected Dependencies (Priority: P1)

As a developer, I want Radius to automatically map detected dependencies to Resource Type definitions (from a pre-defined catalog, with fallback generation for unknown types), so that I don't need to manually define or configure them.

**Why this priority**: Resource Types are essential for Radius to model and deploy applications. Mapping detected dependencies to a curated catalog (with fallback generation) enables seamless adoption while ensuring quality and consistency.

**Independent Test**: Can be tested by verifying that after dependency detection, valid Resource Type definitions are available (from catalog match or generated) that can be used with Radius.

**Acceptance Scenarios**:

1. **Given** detected dependencies from codebase analysis, **When** generation is triggered, **Then** Radius maps each dependency to a Resource Type from the pre-defined catalog.
2. **Given** a detected PostgreSQL dependency, **When** the Resource Type is resolved, **Then** the Resource Type includes appropriate properties (connection string pattern, port, database name).
3. **Given** a detected dependency that is not in the catalog, **When** generation is triggered, **Then** Radius generates a minimal Resource Type and prompts user to contribute it back to the catalog.
4. **Given** an existing Resource Type that matches a detected dependency, **When** generation is triggered, **Then** Radius reuses the existing Resource Type rather than creating a duplicate.

---

### User Story 3 - Match Recipes from Configured Sources (Priority: P2)

As a platform engineer, I want Radius to automatically find and suggest Recipes from both authoritative sources (Azure Verified Modules) and internal repositories (Terraform/Bicep), so that I can deploy applications with production-ready, approved infrastructure patterns.

**Why this priority**: Recipes are what actually provision infrastructure. Supporting both authoritative and internal sources enables adoption in regulated enterprises while leveraging well-tested modules.

**Independent Test**: Can be tested by configuring both AVM and internal sources, then verifying detected dependencies are matched against both catalogs with appropriate ranking.

**Acceptance Scenarios**:

1. **Given** a detected PostgreSQL dependency, **When** Recipe matching is performed, **Then** Radius suggests matching recipes from configured sources (AVM, internal repos).
2. **Given** multiple Recipe options for a dependency, **When** results are presented, **Then** Radius ranks Recipes by relevance and source trustworthiness.
3. **Given** both internal and authoritative sources are configured, **When** matching is performed, **Then** Radius allows the user to prioritize which source takes precedence.
4. **Given** no Recipe matches a detected dependency in any configured source, **When** matching is performed, **Then** Radius indicates no match was found and suggests manual creation.

---

### User Story 4 - Generate Application Definition (Priority: P1)

As a developer, I want Radius to generate a complete application definition that models my application and its infrastructure dependencies, so that I can deploy my application to any environment using Radius.

**Why this priority**: The application definition is the final deliverable that ties together detection, Resource Types, and Recipes. It's what developers actually use to deploy applications.

**Independent Test**: Can be tested by generating an application definition from a sample codebase and verifying it can be used to deploy the application with Radius.

**Acceptance Scenarios**:

1. **Given** analyzed codebase with detected dependencies and matched Recipes, **When** application definition generation is triggered, **Then** Radius produces a valid Bicep application definition file.
2. **Given** a generated application definition, **When** I deploy using Radius, **Then** the application and its infrastructure are successfully provisioned.
3. **Given** a generated application definition, **When** I review it, **Then** I can modify any auto-detected values before deploying.
4. **Given** multiple containers or services in the codebase, **When** generation occurs, **Then** Radius models each service and their interconnections correctly.

---

### User Story 5 - New Application Scaffolding (Priority: P3)

As a developer starting a new application, I want Radius to help me scaffold my application with best-practice infrastructure patterns from the beginning, so that I adopt Radius and cloud-native patterns from day one.

**Why this priority**: While important for greenfield development, the primary value proposition is making adoption trivial for existing applications. New apps benefit once the core discovery mechanism exists.

**Independent Test**: Can be tested by using a scaffolding command to create a new application structure with selected infrastructure dependencies.

**Acceptance Scenarios**:

1. **Given** I specify I need a web application with PostgreSQL and Redis, **When** I run the scaffolding command, **Then** Radius creates an application template with appropriate Resource Types and Recipe references.
2. **Given** a scaffolded application, **When** I add code and deploy, **Then** the infrastructure provisioning works without additional configuration.

---

### User Story 6 - AI Coding Agent Integration (Priority: P1)

As a developer using AI coding agents (GitHub Copilot, Claude, Codex), I want the discovery and generation capabilities exposed as composable skills/tools via MCP (Model Context Protocol), so that AI agents can help me deploy my application through natural conversation.

**Why this priority**: AI-assisted development is becoming the primary way developers interact with tooling. Exposing capabilities as skills enables seamless integration with coding agents, dramatically improving developer experience.

**Independent Test**: Can be tested by invoking each skill via MCP and verifying it returns correct structured output that an AI agent can interpret and act upon.

**Acceptance Scenarios**:

1. **Given** an AI agent with access to Radius skills, **When** a user asks "help me deploy my app to Azure", **Then** the agent can invoke `discover_dependencies` and `discover_services` skills to analyze the workspace.
2. **Given** discovered dependencies, **When** the agent invokes the `generate_recipes` skill, **Then** it receives structured recipe options it can present to the user conversationally.
3. **Given** user confirmation of recipe selections, **When** the agent invokes `generate_app_definition`, **Then** a valid app.bicep file is created in the workspace.
4. **Given** all skills are available via MCP, **When** the CLI commands are invoked, **Then** they use the same underlying skill implementations (shared core).

---

### User Story 7 - Apply Team Infrastructure Practices (Priority: P2)

As a platform engineer, I want Radius to automatically detect and apply my team's infrastructure best practices (naming conventions, tags, cost practices for dev/test/prod, security requirements) when generating Resource Types, so that generated definitions comply with our organizational standards without manual intervention.

**Why this priority**: Enterprise teams have established infrastructure patterns and governance requirements. Automatically applying these practices reduces friction for developers while ensuring compliance with organizational standards.

**Independent Test**: Can be tested by configuring team practices (via config file, detected from existing IaC, or imported from internal wiki) and verifying that generated Resource Types incorporate those practices.

**Acceptance Scenarios**:

1. **Given** a team practices configuration file exists (e.g., `.radius/team-practices.yaml`), **When** discovery is performed, **Then** Radius loads and applies those practices to generation.
2. **Given** existing Terraform or Bicep files in the repository, **When** discovery is performed, **Then** Radius detects naming patterns, tag policies, and sizing configurations from those files.
3. **Given** an internal wiki URL is configured as a practices source, **When** discovery is performed, **Then** Radius extracts infrastructure guidelines from the wiki content.
4. **Given** detected practices include required tags (cost-center, owner, environment), **When** Resource Types are generated, **Then** the Resource Type schemas include those tags as required properties.
5. **Given** environment-specific cost practices exist (dev=Basic tier, prod=Premium tier), **When** generating for a specific environment, **Then** the appropriate tier defaults are applied.
6. **Given** detected practices include a naming convention `{env}-{service}-{resource}`, **When** the app.bicep is generated, **Then** resource names follow that convention.
7. **Given** no team practices are detected or configured, **When** generation occurs, **Then** Radius uses sensible defaults and informs the user they can configure team practices.

---
### Edge Cases

- **Unrecognized dependencies**: System should indicate unknown dependencies and allow manual Resource Type definition.
- **Conflicting library versions**: System should detect the conflict, report it, and allow user to select the intended dependency.
- **Monorepo support**: System should allow specifying the root directory to analyze and support analyzing multiple app roots.
- **Multiple instances of same dependency type** (e.g., two PostgreSQL databases): System should detect unique connection configurations and model them as separate resources.
- **Team practices conflicts** (e.g., different naming patterns in different IaC files): System should detect the conflict, present options to the user, and allow selection.
- **Config vs detected practices precedence**: Explicit configuration takes precedence; system warns about discrepancies.

> **Note**: See FR-10 (partial discovery failures), FR-22 (Recipe source unavailability), FR-20/FR-21 (re-running discovery) for handling of other edge cases.

---

## Data Model

### Key Entities

| Entity | Description | Key Attributes |
|--------|-------------|----------------|
| **Skill** | Composable capability exposed via MCP | name, input/output schema (JSON), description |
| **Detected Dependency** | Infrastructure dependency from codebase analysis | type, technology, confidence score, source evidence |
| **Team Practice** | Infrastructure convention (detected or configured) | category, pattern/value, source, applicable environments |
| **Practices Source** | Location for importing team practices | source type, location/URL, auth method, refresh frequency |
| **Resource Type** | Interface defining what an app consumes from infrastructure | type name, output schema, validation rules |
| **Recipe** | IaC implementation that provisions infrastructure | name, IaC type, source location, parameters, target Resource Type |
| **Recipe Source** | Location for discovering Recipes | source type, location/URL, priority, auth config |
| **Application Definition** | Final Bicep output for deployment | Resource Type references, Recipe bindings |

---

## Requirements *(mandatory)*

### Functional Requirements

Requirements are organized to match the three-phase workflow (Discover → Generate → Deploy) plus Interface Layer (CLI, MCP, API).

#### Phase 1: Discover

*Skills: `discover_dependencies`, `discover_services`, `discover_team_practices`*

| ID | Priority | Requirement |
|----|----------|-------------|
| FR-01 | P1 | System MUST provide a `rad app discover` command that analyzes a local codebase directory. |
| FR-02 | P1 | System MUST detect infrastructure dependencies in Python, JavaScript/TypeScript, Go, Java, and C#. |
| FR-03 | P1 | System MUST detect: databases (PostgreSQL, MySQL, MongoDB, Redis, SQL Server, Cosmos DB), queues (RabbitMQ, Service Bus, Kafka), caches (Redis, Memcached), storage (Blob, S3). |
| FR-04 | P1 | System MUST detect deployable services via entrypoints (main files, Dockerfiles, package.json scripts, framework conventions). |
| FR-05 | P2 | System MUST provide confidence scores: high (≥80%), medium (50-79%), low (<50%), with ≥50% included by default. |
| FR-06 | P1 | System MUST detect team practices from existing IaC files (Terraform, Bicep, ARM) and config file (`.radius/team-practices.yaml`). |
| FR-07 | P2 | System MUST support loading team practices from external documentation sources (wikis, Confluence, Notion, ADRs). |
| FR-08 | P2 | System MUST support environment-specific practices (e.g., dev=Basic tier, prod=Premium+HA). |
| FR-09 | P1 | System MUST output discovery results to `./radius/discovery.md`, including dependencies, services, and team practices with source evidence. |
| FR-10 | P1 | System MUST continue discovery if individual files fail, reporting partial results with warnings. |

#### Phase 2: Generate

*Skills: `generate_resource_types`, `discover_recipes`, `generate_app_definition`, `validate_app_definition`*

| ID | Priority | Requirement |
|----|----------|-------------|
| FR-11 | P1 | System MUST provide a `rad app generate` command that creates a Radius application definition from discovery results. |
| FR-12 | P1 | System MUST generate valid Resource Type definitions for each detected dependency (via pre-defined catalog or generation, per OQ-1). |
| FR-13 | P1 | System MUST apply detected team practices as Resource Type schema defaults and constraints. |
| FR-14 | P1 | System MUST search configured Recipe sources (AVM, internal Terraform, internal Bicep repos) for matching Recipes. |
| FR-15 | P1 | System MUST allow interactive Recipe selection when multiple options are available. |
| FR-16 | P1 | System MUST generate valid `./radius/app.bicep` with containers/services and infrastructure dependencies wired. |
| FR-17 | P1 | System MUST validate generated Bicep: syntax, Resource Type references, and container-to-resource connections. |
| FR-18 | P2 | System MUST reuse existing Resource Types when they match detected dependencies. |
| FR-19 | P2 | When Dockerfiles detected, generated definition MUST reference container image or add TODO placeholder. |
| FR-20 | P2 | System MUST support `--update` flag for diff/patch mode instead of full regeneration. |
| FR-21 | P2 | When existing `app.bicep` found, system MUST prompt: overwrite, merge, show diff, or cancel. |
| FR-22 | P2 | System MUST gracefully degrade when Recipe sources unavailable—mark as "no Recipe found" and continue. |
| FR-23 | P3 | System SHOULD provide `rad app scaffold` for new apps to generate starter app.bicep without existing code. |
| FR-24 | P2 | System MUST support `--add-dependency <type>` to manually specify dependencies without discovery. |

#### Interface Layer (CLI, MCP, API)

*How users invoke the skills*

| ID | Priority | Requirement |
|----|----------|-------------|
| FR-25 | P1 | All capabilities MUST be implemented as composable skills invocable independently. |
| FR-26 | P1 | System MUST expose all skills via MCP server for AI agent integration. |
| FR-27 | P1 | CLI commands MUST be thin wrappers around skills (shared implementation). |
| FR-28 | P1 | Each skill MUST accept JSON input and return JSON output for programmatic consumption. |
| FR-29 | P1 | System MUST provide `rad mcp serve` command to start MCP server. |
| FR-30 | P2 | MCP server MUST support stdio (for VS Code extensions) and HTTP (for remote agents) transports. |
| FR-31 | P2 | MCP server MUST be configurable via flags/env vars for port, allowed origins, auth mode. |

#### Configuration & Options

*Shared settings across phases*

| ID | Priority | Requirement |
|----|----------|-------------|
| FR-32 | P1 | System MUST provide `--accept-defaults` flag for non-interactive/CI usage. |
| FR-33 | P2 | System MUST provide `--recipe-profile` to select environment-specific recipe sets. |
| FR-34 | P2 | System MUST support `--output` flag for custom output paths. |
| FR-35 | P1 | System MUST support recipe source config via `~/.rad/config.yaml` or `.rad/config.yaml`. |
| FR-36 | P1 | System MUST provide `rad recipe source add` to configure new recipe sources. |
| FR-37 | P2 | System MUST support auth to private sources via env vars, credential helpers, or tokens. |

### Non-Functional Requirements

| ID | Category | Requirement |
|----|----------|-------------|
| NFR-01 | Performance | Discovery of ≤100 source files MUST complete within 30 seconds on standard developer hardware. |
| NFR-02 | UX | System MUST provide progress indicators during long-running operations. |
| NFR-03 | UX | All errors MUST include actionable guidance for resolution. |
| NFR-04 | Reliability | Generated app definitions MUST be deterministic (same input → identical output). |
| NFR-05 | Observability | System SHOULD emit structured JSON logs with severity levels. |
| NFR-06 | Scalability | MCP server MUST support concurrent skill invocations without state conflicts. |

---

## Security Considerations

### Credential Handling

- **Recipe Source Authentication**: When accessing private Terraform/Bicep repositories, credentials (tokens, SSH keys) must be stored securely using the system credential store or environment variables. Credentials must never be logged or displayed in CLI output.
- **Generated Secrets**: The generated `app.bicep` must not contain inline secrets. All sensitive values (database passwords, API keys) must reference Radius secret stores or be injected via environment variables at deploy time.
- **MCP Server Security**: The MCP server must validate the origin of requests and may require authentication tokens for non-local connections.

### Codebase Analysis

- **Local-Only Analysis**: Discovery operates only on local filesystem paths provided by the user. No code is transmitted to external services during analysis.
- **No Code Execution**: The discovery phase performs static analysis only. No code from the analyzed codebase is executed.
- **File Access Scope**: Discovery only reads files within the specified directory. It does not follow symlinks outside the project root by default.

### Trust Model

- **Recipe Sources**: Users are responsible for trusting the recipe sources they configure. Recipes from internal sources execute with the same permissions as the Radius deployment.
- **Azure Verified Modules**: AVM recipes are considered trusted as they are maintained by Microsoft with security review processes.
- **User-Provided Inputs**: All user inputs (paths, dependency names, recipe selections) must be validated and sanitized before use.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Developers can go from existing codebase to deployable Radius application in under 30 minutes for applications with up to 5 infrastructure dependencies.
- **SC-002**: The system correctly detects at least 85% of infrastructure dependencies in sample applications from popular frameworks (Express.js, Django, Spring Boot, ASP.NET Core, Gin).
- **SC-003**: Generated application definitions successfully deploy on first attempt for 90% of detected configurations when Recipes are available.
- **SC-004**: Users can complete the entire discovery-to-deploy workflow using only auto-generated configurations for standard infrastructure patterns (no manual Resource Type or Recipe authoring required).
- **SC-005**: Platform engineers can onboard a new internal Recipe source in under 10 minutes.
- **SC-006**: The learning curve for using auto-discovery is reduced such that new users can successfully deploy their first application within 1 hour of installation (compared to current manual approach).
- **SC-007**: Each supported language detects at least the top 5 most common infrastructure libraries for that ecosystem (e.g., for Python: psycopg2, redis-py, boto3, pymongo, azure-storage-blob).

## Assumptions

- Users have access to their application's source code on the local filesystem.
- The codebase is analyzed locally (no remote repository cloning required).
- Azure Verified Modules provide a stable API or catalog for Recipe discovery.
- Internal Terraform and Bicep repositories follow common conventions for module/template organization.
- Users can provide read access to internal module repositories when configuring internal sources.

## Design Constraints

These are intentional design decisions that constrain implementation:

- **DC-001**: Bicep is the target format for generated application definitions (consistent with current Radius conventions).
- **DC-002**: The Radius CLI is the primary user interface for invoking discovery, with MCP and programmatic API as additional consumption layers.
- **DC-003**: Generated application definitions are written to `./radius/app.bicep` by default (overridable via `--output`).
- **DC-004**: The workflow is multi-step (discover → generate → deploy) to allow review and customization between stages.
- **DC-005**: Interactive mode is the default; non-interactive mode is opt-in via `--accept-defaults`.
- **DC-006**: Skills-first architecture - all capabilities are implemented as composable skills, with CLI commands as thin wrappers.

## Open Questions for Discussion and Experimentation

### OQ-1: How should the Application Assembly Layer be designed so that skills and AI agents work together deterministically?

This remains an open area for experimentation. The core challenge is ensuring that when multiple skills are composed (by CLI orchestration or AI agents), the results are reproducible and predictable regardless of the invocation path.

---

## Clarifications (Resolved)

These preliminary clarifications have been discussed and documented to guide implementation:

### Q-1: How should Radius source Resource Types (pre-defined catalog vs dynamic generation) and how should their schemas be determined?

Our approach is to have a predefined catalog of Resource Types maintained in the Resource Types repository. This could be community contributed using verified sources like Azure Verified Modules. For unknown dependencies, we will have fallback generation logic to create the type dynamically and have users confirm and contribute back to the catalog in the Resource Types repository.

For schema definitions of the type, we will use the predefined catalog as the primary source and match the resource type to follow the infrastructure practices detected in the codebase. For unknown dependencies, we will generate a minimal schema with common connection properties from deterministic sources like AVM and allow users to extend them as needed.

### Q-2: How would Radius know the source of truth for team infrastructure practices and IaC configurations?

Automatically detect team practices from existing IaC configurations in the repository first. Store them in the `discovery.md` output for user review. If an external IaC source or an external documentation like an Internal Wiki needs to be added, users can add them directly to the `discovery.md` file.

### Q-3: How should Radius extract infrastructure practices from unstructured documentation (internal wikis, Confluence, Notion)?

Use an LLM to parse wiki content and extract infrastructure practices into a structured format. Define a lightweight structured format (Markdown with specific headings/tables) that teams can adopt in their wikis to facilitate parsing. The LLM will look for these structures to extract practices reliably.

### Q-4: How should Radius access the codebase for analysis?

Radius analyzes codebases from the local filesystem only. The user provides a directory path to the project root. No remote repository cloning or external transmission of code occurs during analysis.

### Q-5: What is the default output location for generated files?

This is dependent on how Radius is installed in a git repository. By default, output to `./radius/discovery.md` for discovery results and `./radius/app.bicep` for the application definition. 

### Q-6: What confidence threshold should be used for including detected dependencies?

Dependencies with ≥50% confidence are included by default. The system visually distinguishes three tiers: high (≥80%), medium (50-79%), and low (<50%). Users can filter or exclude low-confidence dependencies during review.

### Q-7: What should happen when Recipe sources are unavailable or return no matches?

The system continues with graceful degradation. Dependencies without matching Recipes are marked as "no Recipe found" in the output. Users can proceed with partial results and manually specify Recipes later.

### Q-8: How should Radius identify deployable services within a codebase?

Radius uses entrypoint detection to identify deployable services. It looks for main files, Dockerfiles, package.json scripts (such as `start` or `serve`), and framework-specific entrypoints (such as `app.listen()` or `@SpringBootApplication`).

### Q-9: Which language should be the primary focus for initial release?

Use LLMs to read and analyze the codebase, which automatically enables support for all popular programming languages without language-specific analyzers. 
