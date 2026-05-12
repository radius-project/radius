# Radius Resource Types & Recipes

## Summary

As part of GitHub-Radius integration, AI agents need to understand application source code and automatically generate deployment definitions (`app.bicep`). For the agents to work **deterministically and reliably**, they need a well-defined catalog of application-oriented resource types backed by production-ready recipes.

Radius maintains the resource type schemas and recipes in the `resource-type-contrib` repository. 
We need about 30 application oriented resource types covering the basics: databases, caches, messaging, storage, and so on. These are what AI agents use to generate `app.bicep`, so the schemas need to be well-defined. We use  to generate schemas and validate them against community Bicep and Terraform modules.

We don't write recipes. The `resource-type-contrib` repository has type definitions only, no recipe code. Recipes point at community modules directly: Azure Verified Modules for Azure, Terraform Registry for AWS, Helm charts for Kubernetes. Radius handles the input and output mapping automatically with configuration maintained in Recipe packs.

Because we own the type interface but not the module code, there's nothing to maintain, audit, or patch on the recipe side. This also eliminates supply chain concerns since we don't ship or redistribute any IaC code. Bicep and Terraform Recipe drivers work today. Helm is next and will open up Kubernetes coverage significantly. This document lays out the strategy to build and maintain the types and recipes for the GitHub-Radius integration to be successful.

## Goals

1. Build a resource type catalog broad enough for AI agents to generate accurate `app.bicep` for real-world applications.
2. Eliminate recipe authoring by pointing directly at community modules, so Radius never owns or ships IaC code.
3. Establish a contribution model that lets the community add and validate resource types with clear maturity gates from Alpha to Stable.
4. Extend recipe driver coverage to match where developers deploy: Bicep for Azure, Terraform for AWS, Helm for Kubernetes.

## 1. Resource Types

Resource types are the building blocks of the application definition. Today's catalog is limited to a handful of types that serve only the Radius `todo-list` sample. To support real-world applications, the catalog needs to grow to cover the application dependencies developers actually use.

A data-driven analysis by Copilot from cloud provider catalogs, Docker Hub, the Stack Overflow 2025 Developer Survey, IaC registries, and package registry trends identified 27 application components ranked by actual developer adoption. Adoption is measured by dedicated client-library downloads across four ecosystems (npm, PyPI, NuGet, RubyGems), weighted by survey usage, Docker pulls, and cloud availability. The full ranked catalog with methodology is in [`resource-type-ranked-catalog`](2026-05-radius-resource-types-recipes/resource-type-ranked-catalog.md).

The top 27 break into three tiers:

| Tier | What's included | Criteria |
|------|----------------|----------|
| **Build First** | PostgreSQL, Redis, Object Storage, LLM Inference API, MongoDB, MySQL, Kafka, Elasticsearch/OpenSearch, RabbitMQ, SQL Server | Highest adoption + stable connection contracts suitable for cross-cloud abstraction |
| **Build Next** | Serverless Functions, Message Queue, MQTT, pgvector, NATS, Oracle, Neo4j, Vault, Cassandra, InfluxDB | Strong adoption but higher abstraction complexity or narrower use cases |
| **Build Later** | Ollama, Pub/Sub, ClickHouse, Keycloak, Spark, MLflow, Memcached | Emerging, niche, or platform-specific build as demand materializes |

Notable inclusions: LLM Inference API reflects AI becoming a first-class application dependency (81.4% of surveyed developers use OpenAI GPT models). pgvector (#14) is the recommended vector-database entry point with the same PostgreSQL connection contract and 3/3 cloud availability. Vault (#18) is included because applications directly establish runtime connections to secrets providers, unlike org-level identity or observability platforms.

Shared infrastructure services (identity/auth, observability, logging, email, feature flags) are provisioned at the platform level, but applications still connect to them at runtime. These may warrant a lightweight **shared resource type** with no recipe, just connection metadata at the environment level.

### Schema generation

Rather than manually authoring every schema, Radius uses a `resource-type-creator` agent to analyze infrastructure modules, application usage patterns, and existing deployment conventions.

Generated schemas must:

- Expose stable application-facing contracts
- Follow naming and validation conventions
- Avoid provider-specific abstractions

## 2. Recipes

Recipes are how Radius deploys infrastructure behind resource types. Though it is a concept of Radius, the implementation uses existing IaC languages, Bicep and Terraform. Instead of us writing the IaC code, we leverage well established community modules directly. This approach helps Radius to not maintain recipe code reducing the supply chain surface. There is no wrapper code to audit, patch, or keep in sync with upstream module changes.

### Direct Module Support

Today, using a community module as a Radius recipe requires a wrapper that adds a `context` input and a structured `result` output conforming to Radius conventions. This wrapper adds friction, creates maintenance burden to republish to another IaC source and needs constant updates to stay in sync with upstream changes.

Direct Module Support eliminates the Recipe wrapper. Platform engineers point `recipeLocation` at any standard Bicep or Terraform module. Radius handles input resolution (injecting context like resource name and other resource properties into the module's native parameters via `{{context.*}}` expressions) and output resolution (mapping the module's native outputs to resource properties), all externally.

The Recipe Pack bundles recipe definitions for multiple resource types. It maps each type to a module location, handles parameter injection via `{{context.*}}` expressions, and maps module outputs back to resource properties.

```bicep
// RecipePack resource definition
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'azure-production'
  properties: {
    recipes: {
      'Radius.Data/postgreSqlDatabases': {
        recipeKind: 'bicep'
        recipeLocation: 'mcr.microsoft.com/bicep/avm/res/db-for-postgre-sql/flexible-server:0.15.3'
        recipeParameters: {
          name: 'pg-{{context.resource.name}}'
          location: 'westus3'
        }
        outputs: {
          host: 'fqdn'       // module output reference
          database: 'name'    // module output reference
          username: '{{pgadmin}}' // literal value
        }
      }
    }
  }
}

// Environment references the recipe pack
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'my-env'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'my-app-ns'
      }
    }
  }
}
```

For the full technical specification, including user stories, acceptance scenarios, parameter precedence rules, and implementation details, see the [Direct Module Support Spec](https://github.com/Reshrahim/radius/blob/001-direct-module-support/specs/001-direct-module-support/spec.md).

### Recipe module reference generation

Once direct module support is implemented, we create a `recipe-module-generator` agent that tests the type with community modules and builds the recipe pack by platform. 

### Recipe Drivers Coverage

Radius supports Bicep and Terraform drivers today. Bicep is the Azure-native path through AVM, while Terraform provides the broadest AWS and multi-cloud coverage.

Stack Overflow 2025 places Terraform at 17.8% adoption across all respondents, behind Docker (71.1%) and Kubernetes (28.5%) but well ahead of other infrastructure tools like Ansible (11.7%). The survey no longer breaks out Bicep, CloudFormation, or Pulumi individually, having merged IaC into a broader "Cloud development" category.

CNCF 2025 confirms Kubernetes at 82% production adoption among container users, with Helm at 81-87% adoption among Kubernetes organizations. That Helm number is what makes it the highest-leverage next driver for Radius: it maps directly to how the Kubernetes ecosystem already packages and distributes software, and unlocks existing charts instead of requiring custom Bicep modules per backing service.

## 3. Testing and Validation

Resource types and Recipes are tightly coupled and without proper validation and testing, we cannot ensure that the integration works correctly or that the resources are provisioned as expected. Automated tests and validation steps are crucial to maintain reliability and consistency across different modules and recipes.

**Schema Validation** ensures the resource type contract remains stable over time. Validation includes:

- required property and type checks
- output contract validation
- backward compatibility checks across schema versions

**Recipe Validation** ensures upstream infrastructure modules can successfully satisfy the resource type contract. Validation includes:

- successfully deploying referenced modules
- verifying that module outputs map correctly to resource properties
- validating connection generation

## 4. Contributions, Ownership and Lifecycle

With this proposal, the `resource-types-contrib` repository contains only resource type definitions. No recipe code lives there. Hence, contributions will be limited to resource types only and adding tested recipe module references.

Contribution model for adding a new type means:

1. Resource Type schema
2. Documentation and Module references (which AVM/TF/Helm modules to use)
3. Tests that validate the type deploys correctly with those modules

### Maturity Stages

Contributions enter at Alpha or Beta and graduate towards Stable. Stable resource-types are what is added to Radius install.

> **Note:** Resource types that exist in Radius today (e.g., Applications.Core/*, Applications.Data/*) are automatically promoted to Stable once Radius maintainers validate their extensibility equivalents.

**Alpha**

- Schema passes validation (required properties, type checks, output contract)
- `apiVersion` is prefixed as alpha in the type definition
- At least one working recipe module reference on any single platform
- README with usage examples
- Manual testing evidence submitted with the PR
- Maintainer review and approval required before merge

**Beta**

- Recipe module references for all three platforms (AWS, Azure, Kubernetes) in both Bicep and Terraform
- `apiVersion` is prefixed as beta in the type definition
- README with proper usage examples across all platforms
- Recipe packs tested across all supported platforms
- Backward-compatible schema changes
- Maintainer review and approval required before merge

**Stable**

- 100% functional test coverage across all supported modules
- `apiVersion` is prefixed as stable in the type definition
- Full CI/CD integration with release pipeline
- Proven documentation validated by external community members
- Voted by external community members
- Radius core repository maintainer review and approval before merge

**Radius maintainers** are responsible for schema evolution, versioning, compatibility guarantees, deprecation, and promotion through maturity stages for resource types. Resource types are versioned by API (2026-11-05-preview, 2026-11-05-stable), while recipes follow upstream module versioning.

## Action Plan

| Priority | Work Item |
|----------|-----------|
| **P0** | Fix deployment engine bugs with AVM modules |
| **P0** | Resource Type and Recipe schema generation via Agents |
| **P0** | Direct Module Support |
| **P0** | Build Tier 1 Resource type definitions and Recipes |
| **P0** | Alpha maturity gate for Tier 1 |
| **P1** | Test framework for Resource Types and Recipes |
| **P1** | Revamp contrib repository documentation with gates |
| **P1** | Build Tier 2 Resource type definitions and Recipes |
| **P1** | Beta maturity gate for Tier 1, Alpha for Tier 2 |
| **P1** | Helm recipe driver |
| **P2** | Recipe packs (dev-local, azure-prod, aws-prod) |
| **P2** | Shared resource types (connection metadata, no recipe) |
| **P2** | Stable maturity gate for Tier 1 |
| **P3** | CloudFormation driver. native AWS recipe support |
| **P3** | Build Tier 3 Resource type definitions and Recipes |

## Success Metrics

The primary measure is whether the resource type catalog is broad enough for AI agents to generate useful `app.bicep`.

### Resource type and Recipe correctness

- **Dependency resolution rate**: 80%+ of detected app dependencies should map to a defined resource type, measured across common application patterns (web apps, microservices, data pipelines).  
- **Schema accuracy**: Generated `app.bicep` files should require zero manual edits for connection properties (host, port, credentials) when deploying against any supported platform.
- **Deployment success rate**: 100% success rate across all module references, measured by weekly automated test runs against pinned module versions on all three platforms (Azure, AWS, Kubernetes).
- **Breakage detection**: Upstream module changes that break a recipe should be caught with CI runs before they impact deployments.

### Ecosystem Growth

The contribution model covers types only, no recipe code. Success means external contributors find the process accessible and the type catalog grows steadily with community involvement.

- Growing number of external contributions over time (10 per month)
- Repeat contributors returning to add or improve types (at least 2 per month)
- Type catalog expanding across all three tiers

## References

1. [Stack Overflow Developer Survey 2025](https://survey.stackoverflow.co/2025/)
2. [CNCF Annual Cloud Native Survey 2025](https://www.cncf.io/wp-content/uploads/2026/01/CNCF_Annual_Survey_Report_final.pdf)