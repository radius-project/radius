# Feature Specification: Automatic Application Discovery

**Feature Branch**: `001-auto-app-discovery`  
**Created**: January 28, 2026  
**Status**: Draft  
**Input**: User description: "I am building a solution that will make it trivial for developers to adopt Radius for their existing applications and new applications. Radius will remove the need to define Resource Types and Recipes up front. Rather, it will create Resource Types based on understanding of the application's codebase (e.g., the existence of a PostgreSQL library), infrastructure best practices they follow like naming conventions, tags, cost practices for dev/test/prod from their internal wiki or any source, and create Recipes based on either authoritative sources, such as Azure Verified Modules, or internal sources of Terraform modules or Bicep templates. Radius will use these Resource Types and its understanding of the code repo to model the application and generate a rich application definition."

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
┌───────────────────┐      ┌───────────────────┐      ┌───────────────────┐
│      PHASE 1      │ ───▶ │      PHASE 2      │ ───▶ │      PHASE 3      │
│     Discover      │      │      Generate     │      │      Deploy       │
└───────────────────┘      └───────────────────┘      └───────────────────┘
        Skills:                   Skills:                  Existing:
  discover_dependencies     generate (orchestrates):       rad deploy
  discover_services         • generate_resource_types
  discover_team_practices   • discover_recipes
                            • generate_app_definition
                            • validate_app_definition
```

**The logical flow:**
1. **Discover** - Analyze the codebase to detect infrastructure dependencies, deployable services, and team infrastructure practices
2. **Generate** - A single unified step that:
   - Creates Resource Types (the interface/schema) for each detected dependency, incorporating team conventions
   - Discovers IaC implementations (Recipes) from external sources (AVM, Terraform, Bicep repos)
   - Assembles everything into a deployable `app.bicep`
3. **Deploy** - Use existing `rad deploy` to provision infrastructure and run the application

Each phase can be invoked via **AI conversation** or **CLI commands**—both call the same skills.

---

### Skills-First Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                          RADIUS DISCOVERY                                │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│    ┌──────────────────┐   ┌──────────────────┐   ┌──────────────────┐    │
│    │    AI Agents     │   │     rad CLI      │   │  Programmatic    │    │
│    │   (via MCP)      │   │                  │   │      API         │    │
│    │                  │   │                  │   │                  │    │
│    │  "Help me deploy │   │  rad app discover│   │  sdk.Discover()  │    │
│    │   my app..."     │   │  rad app generate│   │  sdk.Generate()  │    │
│    └────────┬─────────┘   └────────┬─────────┘   └────────┬─────────┘    │
│             │                      │                      │              │
│             └──────────────────────┼──────────────────────┘              │
│                                    │                                     │
│                                    ▼                                     │
│    ┌────────────────────────────────────────────────────────────────┐    │
│    │                        SKILLS LAYER                            │    │
│    ├────────────────────────────────────────────────────────────────┤    │
│    │  discover_dependencies │ discover_services │ discover_team_practices│
│    │  discover_recipes │ generate_resource_types │ generate_app_definition│
│    └────────────────────────────────────────────────────────────────┘    │
│                                    │                                     │
│                                    ▼                                     │
│    ┌────────────────────────────────────────────────────────────────┐    │
│    │                        CORE ENGINE                             │    │
│    ├────────────────────────────────────────────────────────────────┤    │
│    │  Language Analyzers │ Team Practices Analyzer │ Bicep Generator│    │
│    └────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

**Available Skills:**

| Skill | Description | Returns |
|-------|-------------|---------|
| `discover_dependencies` | Analyze codebase for infrastructure dependencies | `{dependencies: [{type, technology, confidence, evidence}]}` |
| `discover_services` | Find deployable services/entrypoints | `{services: [{name, type, port, entrypoint}]}` |
| `discover_team_practices` | Detect team infrastructure conventions from config files, existing IaC, internal documentation (wikis, ADRs), and naming patterns | `{practices: [{category, convention, source, environment, examples}]}` |
| `generate_resource_types` | Create Resource Type definitions (interface/schema) for dependencies, applying team conventions | `{resourceTypes: [{name, schema, outputs}]}` |
| `discover_recipes` | Find IaC implementations from external sources that satisfy Resource Types | `{recipes: [{resourceType, name, source, iacType, parameters}]}` |
| `generate_app_definition` | Assemble everything into a Radius app.bicep | `{path: string, content: string}` |
| `validate_app_definition` | Validate generated Bicep | `{valid: boolean, errors: []}` |

---

### Phase 1: Discover Your Application

**What happens**: Radius analyzes your codebase to identify infrastructure dependencies (databases, caches, queues, storage), deployable services (containers, entrypoints), and team infrastructure practices (naming conventions, tags, cost practices for dev/test/prod, security patterns) from config files, existing IaC, or internal documentation.

**Skills invoked**: `discover_dependencies`, `discover_services`, `discover_team_practices`

**CLI Command**: `rad app discover .`

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

Run 'rad app generate' to create your 
application definition.
```

</td>
</tr>
</table>

**Output**: A discovery report (JSON) listing detected dependencies, services, and team practices.

---

### Phase 2: Generate Application

**What happens**: A single unified generation step that:
1. **Creates Resource Types** - Generates the interface/schema for each detected dependency, applying team conventions (naming, sizing, security defaults)
2. **Discovers Recipes** - Searches external sources (AVM, Terraform repos, Bicep templates) for IaC implementations that match team practices
3. **Lets user select Recipes** - Presents options and confirms selections
4. **Generates app.bicep** - Assembles services, Resource Types, and Recipes into a deployable application definition
5. **Validates** - Ensures the generated Bicep is syntactically correct

**Skills invoked**: `generate_resource_types`, `discover_recipes`, `generate_app_definition`, `validate_app_definition`

**CLI Command**: `rad app generate`

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

### Phase 3: Deploy

**What happens**: Standard Radius deployment. The generated `app.bicep` is deployed like any other Radius application.

**Command**: `rad deploy` (existing Radius functionality—no new skills needed)

**CLI Command**: `rad deploy ./radius/app.bicep -e <environment>`

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

### Alternative Workflows

These variations work identically via AI or CLI—just different invocation styles.

**Non-Interactive Mode (CI/CD)**:
```bash
# CLI: Accept all defaults for automation
rad app discover . --output discovery.json
rad app generate --input discovery.json --accept-defaults --output app.bicep
rad deploy app.bicep -e production

# AI: "Discover, generate with defaults, and deploy to production—no prompts"
```

**Environment-Specific Profiles**:
```bash
# CLI
rad app generate --recipe-profile development  # Uses containers
rad app generate --recipe-profile production   # Uses managed services

# AI: "Generate for production" vs "Generate for local development"
```

**Skip Discovery (Known Dependencies)**:
```bash
# CLI
rad app generate --add-dependency postgresql --add-dependency redis

# AI: "I need PostgreSQL and Redis—skip scanning and just generate"
```

---

### Complete Flow Diagram

```
     USER (AI or CLI)                    RADIUS                    EXTERNAL SOURCES
           │                               │                              │
           │  Phase 1: Discover            │                              │
           │──────────────────────────────▶│                              │
           │                               │                              │
           │                     ┌─────────┴─────────┐                    │
           │                     │ discover_dependencies                  │
           │                     │ discover_services  │                   │
           │                     │ discover_team_practices                │
           │                     └─────────┬─────────┘                    │
           │                               │                              │
           │  Discovery Report (JSON)      │                              │
           │◀──────────────────────────────│                              │
           │                               │                              │
           │  Phase 2: Generate            │                              │
           │──────────────────────────────▶│                              │
           │                               │                              │
           │                     ┌─────────┴─────────┐                    │
           │                     │generate_resource_types                 │
           │                     │ (applies team practices)               │
           │                     └─────────┬─────────┘                    │
           │                               │                              │
           │                     ┌─────────┴─────────┐  Query Sources     │
           │                     │  discover_recipes │─────────────────────▶
           │                     │ • Azure Verified  │  • AVM Registry    │
           │                     │ • Internal repos  │◀─• Terraform Repos │
           │                     └─────────┬─────────┘  • Bicep Templates │
           │                               │                              │
           │  Recipe Options               │                              │
           │◀──────────────────────────────│                              │
           │                               │                              │
           │  User Selections              │                              │
           │──────────────────────────────▶│                              │
           │                               │                              │
           │                     ┌─────────┴─────────┐                    │
           │                     │generate_app_definition                 │
           │                     │validate_app_definition                 │
           │                     └─────────┬─────────┘                    │
           │                               │                              │
           │  ./radius/app.bicep           │                              │
           │◀──────────────────────────────│                              │
           │                               │                              │
           │  Phase 3: Deploy              │                              │
           │──────────────────────────────▶│                              │
           │                               │                              │
           │                     ┌─────────┴─────────┐  Provision:        │
           │                     │   rad deploy      │─────────────────────▶
           │                     │ (existing Radius) │  • Azure PostgreSQL│
           │                     │                   │◀─• Azure Redis     │
           │                     └─────────┬─────────┘  • Storage Account │
           │                               │                              │
           │  ✅ Deployment Complete!      │                              │
           │◀──────────────────────────────│                              │
           ▼                               ▼                              ▼
```

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

> **Note**: This user story's implementation depends on the resolution of **OQ-1** (Pre-defined vs Generated Resource Types). If Option A (pre-defined catalog) is chosen, this story becomes "map detected dependencies to existing Resource Types" rather than "generate new Resource Types."

As a developer, I want Radius to automatically provide Resource Type definitions for the dependencies it detects in my codebase, so that I don't need to manually define or configure them.

**Why this priority**: Resource Types are essential for Radius to model and deploy applications. Providing them automatically from detected dependencies directly enables the seamless adoption experience.

**Independent Test**: Can be tested by verifying that after dependency detection, valid Resource Type definitions are available (either from catalog or generated) that can be used with Radius.

**Acceptance Scenarios**:

1. **Given** detected dependencies from codebase analysis, **When** generation is triggered, **Then** Radius creates valid Resource Type definitions for each detected dependency type.
2. **Given** a detected PostgreSQL dependency, **When** the Resource Type is generated, **Then** the Resource Type includes appropriate properties (connection string pattern, port, database name).
3. **Given** an existing Resource Type that matches a detected dependency, **When** generation is triggered, **Then** Radius reuses the existing Resource Type rather than creating a duplicate.

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

> **Note**: See FR-027 (partial discovery failures), FR-019 (Recipe source unavailability), FR-032/FR-033 (re-running discovery) for handling of other edge cases.

---

## Data Model

### Key Entities

- **Skill**: A composable, independently-invocable capability exposed via MCP. Includes: skill name, input schema (JSON), output schema (JSON), description for AI agents. Core skills: `discover_dependencies`, `discover_services`, `discover_team_practices`, `discover_recipes`, `generate_resource_types`, `generate_app_definition`, `validate_app_definition`.
- **Detected Dependency**: Represents an infrastructure dependency identified from codebase analysis. Includes: dependency type (database, cache, queue, etc.), specific technology (PostgreSQL, Redis, etc.), confidence score, and source evidence (file path, library name).
- **Team Practice**: A detected or configured infrastructure convention. Includes: category (naming, tags, cost-optimization, sizing, security, availability), pattern/value, source (config file, IaC detection, internal wiki/documentation), applicable environments (dev/test/prod or all), and examples found. Supports environment-specific variations (e.g., dev uses Basic tier, prod uses Premium).
- **Practices Source**: A configured location from which team practices can be imported. Includes: source type (config file, internal wiki, Confluence, Notion, ADR repository), location/URL, authentication method, and refresh frequency.
- **Resource Type**: The interface/schema that defines what properties an application can consume from an infrastructure resource. Includes: type name (e.g., `Applications.Datastores/postgreSqlDatabases`), output schema (connectionString, host, port, etc.), and validation rules. Resource Types are the abstraction layer between apps and infrastructure. Team practices are applied as schema defaults and constraints.
- **Recipe**: The IaC implementation that provisions infrastructure and satisfies a Resource Type's contract. Includes: recipe name, IaC type (Terraform or Bicep), source location (AVM, internal repo), input parameters, and the Resource Type it implements. Recipes contain the actual provisioning code.
- **Recipe Source**: A configured location where Recipes can be discovered. Includes: source type (Azure Verified Modules, internal Terraform, internal Bicep), location/URL, priority, and authentication configuration.
- **Application Model**: The generated representation of the application. Includes: application name, containers/services, resource references (by Resource Type), and connections between components.
- **Application Definition**: The final Bicep file output representing the complete Radius application ready for deployment. References Resource Types and specifies which Recipes to use for each.

---

## Requirements *(mandatory)*

### Functional Requirements

#### Discovery

- **FR-001**: System MUST provide a `rad app discover` command that analyzes a local codebase directory to detect infrastructure dependencies.
- **FR-002**: System MUST analyze codebases in common programming languages (Python, JavaScript/TypeScript, Go, Java, C#) to detect infrastructure dependencies, with all 5 languages supported at equal priority from initial release.
- **FR-003**: System MUST detect common infrastructure dependencies including: databases (PostgreSQL, MySQL, MongoDB, Redis, SQL Server, Cosmos DB), message queues (RabbitMQ, Azure Service Bus, Kafka), caches (Redis, Memcached), and object storage (Azure Blob Storage, AWS S3).
- **FR-014**: System MUST provide confidence scores for detected dependencies using three tiers: high (≥80%), medium (50-79%), and low (<50%), with only dependencies ≥50% included in reports by default.
- **FR-020**: System MUST identify deployable services by detecting entrypoint patterns including: main files (main.go, main.py, index.js), Dockerfiles, package.json scripts (start, serve), and framework-specific entrypoints (ASP.NET Program.cs, Spring Boot @SpringBootApplication).
- **FR-026**: System MUST detect existing Dockerfiles and extract container image names/build contexts for use in the generated application definition.
- **FR-027**: System MUST continue discovery even if individual files fail to parse, reporting partial results with warnings for unparseable files.

#### Team Infrastructure Practices

- **FR-037**: System MUST provide a `discover_team_practices` skill that detects team infrastructure conventions from the codebase and configured documentation sources.
- **FR-038**: System MUST detect team practices from existing IaC files (Terraform `.tf`, Bicep `.bicep`, ARM templates) by extracting naming patterns, sizing defaults, tags, and configuration values.
- **FR-039**: System MUST support loading team practices from a configuration file (`.radius/team-practices.yaml` or `~/.rad/team-practices.yaml`) with the following categories: naming conventions, sizing defaults, security requirements, HA configurations, tag policies, and cost optimization rules.
- **FR-040**: System MUST apply detected team practices when generating Resource Types, incorporating conventions as schema defaults and constraints.
- **FR-041**: System MUST present detected team practices to the user during discovery, allowing them to confirm, modify, or ignore specific practices.
- **FR-042**: When multiple sources of team practices exist (config file, detected from IaC, documentation), system MUST merge them with explicit configuration taking precedence over detected patterns.
- **FR-043**: System MUST support team practice categories including: naming conventions (patterns for resource names), sizing (min/max values, default tiers), security (encryption, network isolation, authentication requirements), availability (HA, geo-redundancy, backup policies), tagging (required tags, auto-generated tags), and cost optimization (dev/test/prod tier recommendations, shutdown schedules, reserved capacity).
- **FR-044**: System MUST support configuring external documentation sources (internal wiki URLs, Confluence pages, ADR repositories, Notion pages) from which team practices can be extracted.
- **FR-045**: System MUST support environment-specific practices (e.g., dev uses Basic tier, prod uses Premium with HA) and apply the appropriate practices based on the target deployment environment.

#### Recipe Matching

- **FR-006**: System MUST search Azure Verified Modules for matching Recipes when configured.
- **FR-007**: System MUST support configuring internal Terraform module repositories as Recipe sources.
- **FR-008**: System MUST support configuring internal Bicep template repositories as Recipe sources.
- **FR-010**: System MUST allow users to interactively select Recipes when multiple options are available.
- **FR-019**: System MUST use graceful degradation when Recipe sources are unavailable - continue the workflow, mark affected dependencies as "no Recipe found", and allow users to proceed or manually specify Recipes.
- **FR-028**: System MUST support recipe source configuration via a configuration file (`~/.rad/config.yaml` or project-level `.rad/config.yaml`) with the following properties per source: type, URL/path, priority, and authentication method.
- **FR-029**: System MUST support authentication to private recipe sources via: environment variables, credential helpers, or explicit token configuration.
- **FR-030**: System MUST provide a `rad recipe source add` command to interactively configure new recipe sources.

#### Generation

- **FR-004**: System MUST provide a `rad app generate` command that creates a Radius application definition based on discovered dependencies.
- **FR-005**: System MUST provide valid Resource Type definitions for detected dependencies (via pre-defined catalog or generation, per OQ-1 resolution).
- **FR-009**: System MUST generate a valid Radius application definition (Bicep format) to `./radius/app.bicep` that includes detected containers/services and their infrastructure dependencies.
- **FR-013**: System MUST allow users to review and modify auto-detected Resource Types and Recipes before finalizing.
- **FR-015**: System MUST reuse existing Resource Types when they match detected dependencies rather than creating duplicates.
- **FR-017**: System MUST generate application definitions that are compatible with existing Radius deployment workflows (`rad deploy`).
- **FR-031**: When Dockerfiles are detected, the generated application definition MUST reference the appropriate container image (either from Dockerfile image name or a placeholder with TODO comment for user to specify).
- **FR-032**: System MUST support an `--update` flag that compares current discovery results with existing `app.bicep` and generates a diff/patch rather than full regeneration.
- **FR-033**: When an existing `app.bicep` is detected, system MUST prompt user to choose: overwrite, merge (interactive), show diff, or cancel.

#### Skills & MCP Integration

- **FR-021**: System MUST implement all discovery and generation capabilities as composable skills/tools that can be invoked independently.
- **FR-022**: System MUST expose all skills via MCP (Model Context Protocol) server for AI coding agent integration.
- **FR-023**: CLI commands (`rad app discover`, `rad app generate`) MUST be thin wrappers around the underlying skills, sharing the same implementation.
- **FR-024**: Each skill MUST accept structured input (JSON) and return structured output (JSON) suitable for programmatic consumption by AI agents.
- **FR-025**: System MUST provide these core skills: `discover_dependencies`, `discover_services`, `discover_team_practices`, `discover_recipes`, `generate_resource_types`, `generate_app_definition`, `validate_app_definition`.
- **FR-034**: System MUST provide a `rad mcp serve` command that starts the MCP server as a long-running process for AI agent connections.
- **FR-035**: The MCP server MUST be configurable via command-line flags or environment variables for: port, allowed origins, and authentication mode.
- **FR-036**: The MCP server MUST support stdio transport (for VS Code extensions) and HTTP transport (for remote agents).

#### CLI Options & Modes

- **FR-011**: System MUST provide a `--accept-defaults` flag for non-interactive/CI/CD usage.
- **FR-012**: System MUST provide `--recipe-profile` option to select environment-specific recipe sets (e.g., development uses containers, production uses managed services). Default profiles ship with Radius; users can override via a config file.
- **FR-016**: System MUST support both interactive (CLI) and programmatic (API) modes of operation.
- **FR-018**: System MUST support `--output` flag to specify custom output paths for discovery results and generated definitions.

### Non-Functional Requirements

- **NFR-001**: Discovery of a codebase with up to 100 source files MUST complete within 30 seconds on standard developer hardware.
- **NFR-002**: The system MUST provide clear progress indicators during long-running operations (discovery, recipe matching, generation).
- **NFR-003**: All errors MUST include actionable guidance for resolution (e.g., "Recipe source unavailable - check network connectivity or configure offline mode").
- **NFR-004**: The MCP server MUST support concurrent skill invocations from multiple AI agents without state conflicts.
- **NFR-005**: Generated application definitions MUST be deterministic - the same input codebase with the same recipe selections MUST produce identical output.
- **NFR-006**: The system SHOULD emit structured logs suitable for debugging and observability (JSON format, with severity levels).
- **NFR-007**: The system MUST gracefully handle partial failures during discovery, continuing to analyze remaining files and producing partial results with clear warnings.
- **NFR-008**: Credentials for private recipe sources MUST NOT be logged, displayed in output, or included in generated files.
- **NFR-009**: The MCP server MUST validate all incoming requests and reject malformed or unauthorized requests with appropriate error responses.
- **NFR-010**: Generated application definitions MUST NOT contain hardcoded secrets; connection strings and credentials MUST be referenced via Radius secret stores or environment variables.

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
- **SC-007**: At least 3 supported languages have comprehensive dependency detection coverage (Python, JavaScript/TypeScript, and one of Go/Java/C#).

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

## Open Questions for Discussion

These architectural decisions require team discussion before implementation planning.

### OQ-1: Pre-defined vs Generated Resource Types

**Question**: Should Radius ship with a pre-defined catalog of Resource Types (aligned to Azure Verified Modules), or should Resource Types be generated dynamically based on detected dependencies?

**Context**: Mark's guidance is that the more deterministic we can make Radius, the better. Currently FR-005 says "generate valid Resource Type definitions based on detected dependencies."

| Option | Description | Trade-offs |
|--------|-------------|------------|
| **A - Pre-defined catalog** | Ship Radius with a curated catalog of Resource Types for common infrastructure (PostgreSQL, Redis, Cosmos DB, Service Bus, etc.). Discovery maps detected dependencies to existing types. | ✓ Deterministic ✓ Tested ✓ Consistent across users. ✗ Limited to catalog ✗ Requires Radius releases to add new types |
| **B - Generated on-the-fly** | Radius generates Resource Type schemas dynamically based on what it discovers in the codebase | ✓ Flexible ✓ Handles unknown types. ✗ Non-deterministic ✗ Schema quality varies ✗ Harder to test |
| **C - Hybrid approach** | Pre-defined catalog for known types, with fallback generation for unrecognized dependencies | ✓ Best of both worlds. ✗ More implementation complexity ✗ Two code paths to maintain |

**Recommendation**: Option A aligns with determinism guidance. The catalog grows with each Radius release.

**Impact if unresolved**: Affects FR-005, User Story 2, and overall architecture of the generation phase.

---

### OQ-2: Resource Type Schema Design

**Question**: How should Resource Type schemas be determined? What properties should they expose?

**Context**: Resource Types define the contract between infrastructure (Recipes) and applications. The schema determines what connection information is available to apps.

| Option | Description | Trade-offs |
|--------|-------------|------------|
| **A - Derived from AVM outputs** | Each Resource Type schema mirrors the outputs of its corresponding Azure Verified Module (connection strings, endpoints, keys, resource IDs) | ✓ Direct mapping to AVM ✓ Full Azure capabilities. ✗ Azure-specific ✗ Not portable across cloud providers |
| **B - Standardized by category** | All databases share a common schema (connectionString, host, port, database, username, password); all caches share another; all queues share another | ✓ Portable ✓ Predictable ✓ Easy for app developers. ✗ May lose cloud-specific features ✗ Lowest common denominator |
| **C - Minimal portable + extensions** | Core portable schema for common properties, with optional cloud-specific extensions (e.g., `azure.resourceId`, `aws.arn`) | ✓ Portable by default ✓ Cloud features available when needed. ✗ Schema complexity ✗ Apps must handle optional fields |

**Recommendation**: Option B or C - schemas should reflect what *apps* need (connection info), not what *infrastructure* provides.

**Impact if unresolved**: Affects all Resource Type definitions, Recipe output contracts, and app portability story.

---

### OQ-3: Existing IaC and Deployment Scripts

**Question**: Should Radius detect and utilize existing Infrastructure-as-Code (Terraform, Bicep, Helm) or deployment scripts already present in the repository?

**Context**: Many existing applications already have IaC in `/infra`, `/terraform`, or similar directories. Should discovery incorporate this?

| Option | Description | Trade-offs |
|--------|-------------|------------|
| **A - Parse and incorporate** | Detect existing IaC, extract resource definitions, and incorporate them into the generated Radius app definition | ✓ Leverages existing work ✓ Preserves customizations. ✗ Very complex to parse arbitrary TF/Bicep ✗ Error-prone ✗ Large scope |
| **B - Inform only** | Detect existing IaC and report it to the user ("Found existing Terraform in /infra - consider migrating"), but don't auto-incorporate | ✓ Useful context ✓ Low complexity ✓ User decides. ✗ Doesn't reduce manual work |
| **C - Ignore for v1** | Focus solely on application code analysis; treat existing IaC as out-of-scope for initial release | ✓ Simplest ✓ Faster to ship. ✗ Misses opportunity ✗ May duplicate existing infra |

**Recommendation**: Option B for v1 - inform users about existing IaC so they can make informed decisions. Consider Option A for future versions.

**Impact if unresolved**: Affects scope of discovery phase and user expectations for existing projects.

---

### OQ-4: Container Image Strategy

**Question**: How should Radius handle container images for detected services when generating the application definition?

**Context**: The generated `app.bicep` needs container image references for each service. The codebase may or may not have Dockerfiles, and images may or may not be pre-built.

| Option | Description | Trade-offs |
|--------|-------------|------------|
| **A - Dockerfile detection** | If a Dockerfile exists, extract or infer the image name; otherwise, generate a placeholder with TODO comment | ✓ Works with existing Docker workflows ✓ Clear guidance for users. ✗ Doesn't help users without Dockerfiles |
| **B - Image registry inference** | Attempt to infer image names from common patterns (project name, registry conventions) | ✓ More automated. ✗ Often wrong ✗ Magic behavior |
| **C - Always placeholder** | Always generate placeholders requiring user to specify image names | ✓ Explicit ✓ No wrong guesses. ✗ More manual work ✗ Can't deploy without editing |
| **D - Build integration** | Integrate with container build (detect Dockerfile, offer to build and push as part of workflow) | ✓ Complete workflow. ✗ Large scope ✗ Requires registry access |

**Recommendation**: Option A for v1 - detect Dockerfiles when present, placeholder otherwise.

**Impact if unresolved**: Affects FR-031 implementation and user experience during generation.

---

### OQ-5: MCP Server Deployment Model

**Question**: How should the MCP server be deployed and accessed by AI agents?

**Context**: AI agents (GitHub Copilot, Claude) need to connect to the MCP server to invoke skills. The connection model affects security, setup complexity, and user experience.

| Option | Description | Trade-offs |
|--------|-------------|------------|
| **A - Embedded in rad CLI** | `rad mcp serve` starts server; AI agents connect via stdio or local port | ✓ Simple setup ✓ Inherits user auth context. ✗ Must be running ✗ Per-user instance |
| **B - VS Code extension** | Ship a VS Code extension that bundles the MCP server | ✓ Seamless for VS Code users ✓ Auto-starts. ✗ VS Code only ✗ Separate distribution |
| **C - Standalone daemon** | Install MCP server as a system service | ✓ Always available ✓ Shared instance. ✗ More complex setup ✗ Security considerations |
| **D - Remote/cloud hosted** | Radius project hosts a shared MCP server | ✓ Zero setup. ✗ Requires sending code context ✗ Privacy concerns ✗ Operational burden |

**Recommendation**: Option A (embedded in CLI) with Option B as future enhancement for VS Code users.

**Impact if unresolved**: Affects FR-034, FR-035, FR-036 implementation and distribution strategy.

---

### OQ-6: Recipe Source Configuration UX

**Question**: What is the configuration format and workflow for setting up recipe sources (especially internal repositories)?

**Context**: Enterprise users need to configure access to private Terraform/Bicep repositories. The configuration UX affects adoption friction.

| Option | Description | Trade-offs |
|--------|-------------|------------|
| **A - Config file only** | Users manually edit `~/.rad/config.yaml` with source definitions | ✓ Scriptable ✓ Version-controllable. ✗ Error-prone ✗ Discovery of options is hard |
| **B - Interactive CLI** | `rad recipe source add` walks through configuration interactively | ✓ Guided experience ✓ Validation. ✗ Not scriptable |
| **C - Both** | Support both config file and interactive CLI, with CLI writing to config file | ✓ Best of both ✓ Users choose workflow. ✗ Two code paths |
| **D - Environment variables only** | Configure sources via `RAD_RECIPE_SOURCE_*` environment variables | ✓ CI/CD friendly. ✗ Awkward for local dev ✗ Limited structure |

**Recommendation**: Option C - config file as source of truth, with interactive CLI for guided setup.

**Impact if unresolved**: Affects FR-028, FR-029, FR-030 implementation and enterprise adoption.

---

### OQ-7: Team Practices Detection Scope

**Question**: What sources should Radius analyze to detect team infrastructure practices, and how deeply should it parse them?

**Context**: The requirement specifies detecting "infrastructure best practices they follow like naming conventions, tags, cost practices for dev/test/prod from their internal wiki or any source." This requires supporting multiple source types.

| Option | Description | Trade-offs |
|--------|-------------|------------|
| **A - Config file only** | Team practices must be explicitly configured in `.radius/team-practices.yaml`; no auto-detection | ✓ Explicit ✓ Deterministic ✓ Simple to implement. ✗ Manual setup required ✗ May not reflect actual practices |
| **B - IaC detection only** | Scan existing IaC files for naming patterns, tags, sizing values without external sources | ✓ Works offline ✓ No auth needed. ✗ Misses documented practices ✗ Limited to code patterns |
| **C - IaC + Documentation** | Detect from IaC files AND support configured documentation sources (wikis, ADRs, Notion) | ✓ Comprehensive ✓ Captures documented guidelines. ✗ Requires API integrations ✗ Natural language parsing complexity |
| **D - Full hybrid** | Config file for explicit, IaC detection for patterns, documentation for guidelines, with precedence rules | ✓ Most complete ✓ Flexible for different teams. ✗ Complex implementation ✗ Many integration points |

**Recommendation**: Option C for v1 - support IaC detection plus pluggable documentation sources. Start with Confluence and Notion APIs, with extensibility for other sources.

**Impact if unresolved**: Affects FR-037, FR-044, FR-045 implementation and the scope of external integrations.

---

### OQ-8: Documentation Source Parsing Strategy

**Question**: How should Radius extract infrastructure practices from unstructured documentation (internal wikis, Confluence, Notion)?

**Context**: Teams often document infrastructure guidelines in wikis rather than machine-readable formats. Extracting practices from natural language documentation is challenging.

| Option | Description | Trade-offs |
|--------|-------------|------------|
| **A - Structured format required** | Require teams to document practices in a specific format (YAML, JSON, or structured Markdown template) | ✓ Reliable parsing ✓ No ambiguity ✓ Simple to implement. ✗ Requires teams to adopt new format ✗ Existing docs not usable |
| **B - LLM-assisted extraction** | Use an LLM to parse wiki content and extract infrastructure practices | ✓ Works with existing docs ✓ Handles natural language. ✗ Non-deterministic ✗ Requires LLM access ✗ Cost/latency |
| **C - Pattern-based extraction** | Use regex/heuristics to find common patterns (tables, headers like "Naming Convention", lists) | ✓ Deterministic ✓ Works offline ✓ No LLM needed. ✗ Fragile ✗ Misses unstructured content |
| **D - Hybrid** | Pattern-based first, with optional LLM enhancement for ambiguous content | ✓ Best accuracy ✓ Graceful degradation. ✗ Two code paths ✗ Complexity |

**Recommendation**: Option A for v1 - define a lightweight structured format (Markdown with specific headings/tables) that teams can adopt in their wikis. Consider Option D for future versions.

**Impact if unresolved**: Affects FR-044 implementation and determines whether wiki integration is practical for v1.

---

## Clarifications

### Session 2026-01-28

- Q: How should Radius access the codebase for analysis? → A: Local filesystem only - user provides a directory path
- Q: What is the default output location for generated files? → A: `./radius/app.bicep`
- Q: Should the workflow be single command or multi-step? → A: Multi-step (discover, generate, deploy) for transparency
- Q: Should interactive mode be default? → A: Yes, with `--accept-defaults` for CI/CD
- Q: What confidence threshold should be used for including detected dependencies? → A: Show all ≥50% confidence, visually distinguish high (≥80%), medium (50-79%), and low (<50%) tiers
- Q: What should happen when Recipe sources are unavailable or return no matches? → A: Graceful degradation - continue but mark dependencies as "no Recipe found", let user proceed or manually specify
- Q: How should Radius identify deployable services within a codebase? → A: Entrypoint detection - look for main files, Dockerfiles, package.json scripts, or framework-specific entrypoints
- Q: Where should recipe profiles (development, production) be defined? → A: Built-in with override - default profiles ship with Radius, users can override via config file
- Q: Which language should be the primary focus for initial release? → A: Equal priority - all 5 languages (Python, JS/TS, Go, Java, C#) at same priority from start
- Q: Should the feature be architected as composable skills/tools for AI agent integration? → A: Skills-first architecture - build as composable tools, CLI wraps them, expose via MCP for AI agents
- Q: What is the relationship between Resource Types and Recipes? → A: **Resource Types are the interface/schema** (the abstraction defining what properties apps consume); **Recipes are the implementation** (the actual IaC code - Terraform/Bicep - that provisions infrastructure). This separation enables portability across environments.
- Q: Should Radius incorporate team infrastructure practices when generating Resource Types? → A: **Yes** - see OQ-7 and OQ-8 for detailed options on detection scope and documentation parsing.
