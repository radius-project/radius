# Radius Resource Types & Recipes

* **Author**: Reshma Abdul Rahim (@Reshrahim)

## Summary

As part of GitHub-Radius integration, AI agents need to understand application source code and automatically generate deployment definitions (`app.bicep`). For the agents to work **deterministically and reliably**, they need a well-defined catalog of application-oriented resource types backed by production-ready recipes.

Radius maintains the resource type schemas and recipes in the `resource-types-contrib` repository. We need about 30 application oriented resource types covering the basics: databases, caches, messaging, storage, and so on. These are what AI agents use to generate `app.bicep`, so the schemas need to be well-defined.

Recipes reference community modules directly rather than custom IaC code. The `resource-types-contrib` repository contains type definitions and tested module references in Recipe packs. For Azure, Recipes point at Azure Verified Modules. For AWS, the Terraform Registry. and for Kubernetes in the future points to Helm charts. Radius resolves inputs and outputs automatically, with the mapping configuration maintained in Recipe packs.

In most cases, there is no IaC code to maintain in the `resource-types-contrib` repository where only IaC module references are maintained in the Recipe packs for the individual types. In rare cases where no verified community module exists or where Radius-specific orchestration is needed (e.g., the container recipe), a manually written Recipe is still required. This document lays out the strategy to build and maintain the Resource types and Recipes for the GitHub-Radius integration to be successful.

## Goals

1. Build the Radius Resource Type catalog broad enough for AI agents to generate accurate `app.bicep` for real-world applications.
2. Minimize Recipe authoring by pointing directly at community modules(Azure Verified Modules for Azure, Terraform modules for AWS) where possible. Custom recipe code is only needed when no suitable upstream module exists or Radius-specific orchestration is required.
3. Establish a contribution model that lets the community add and validate resource types with clear maturity gates from Alpha to Stable.
4. Extend Recipe driver coverage to match where developers deploy: Bicep/Terraform for Azure (via AVM), Terraform for AWS, Helm for Kubernetes.

## 1. Resource Types

Resource types are the building blocks of the application definition. Today's catalog is limited to a handful of types that serve only the Radius `todo-list` sample. To support real-world applications, the catalog needs to grow to cover the application dependencies developers actually use.

A data-driven analysis by Copilot from cloud provider catalogs, Docker Hub, the Stack Overflow 2025 Developer Survey, IaC registries, and package registry trends identified 27 application components ranked by actual developer adoption. Adoption is measured by dedicated client-library downloads across four ecosystems (npm, PyPI, NuGet, RubyGems), weighted by survey usage, Docker pulls, and cloud availability. The full ranked catalog with methodology is in [`resource-type-ranked-catalog`](2026-05-radius-resource-types-recipes/resource-type-ranked-catalog.md).

The top 27 break into three tiers:

| Tier | What's included | Criteria |
|------|----------------|----------|
| **Build First** | PostgreSQL, Redis, Object Storage, OpenAI-compatible API, MongoDB, MySQL, Kafka, Elasticsearch/OpenSearch, RabbitMQ, SQL Server | Highest adoption + stable connection contracts suitable for cross-cloud abstraction |
| **Build Next** | Serverless Functions, Message Queue (SQS/Azure Queue/Service Bus Queues), MQTT, pgvector, NATS, Oracle, Neo4j, Vault, Cassandra, InfluxDB | Strong adoption but higher abstraction complexity or narrower use cases |
| **Build Later** | Ollama, Pub/Sub, ClickHouse, Keycloak, Spark, MLflow, Memcached | Emerging, niche, or platform-specific — build as demand materializes |

Notable inclusions: OpenAI-compatible API reflects AI becoming a first-class application dependency (81.4% of surveyed developers use OpenAI GPT models). The resource type represents the chat/completions API contract implemented by OpenAI, Azure OpenAI, Anthropic, and compatible providers (Ollama, vLLM). pgvector is the recommended vector-database entry point with the same PostgreSQL connection contract and 3/3 cloud availability. Vault is included because applications directly establish runtime connections to secrets providers, unlike org-level identity or observability platforms.

Shared infrastructure services (identity/auth, observability, logging, email, feature flags) are provisioned at the platform level, but applications still connect to them at runtime. For these, the environment provides connection metadata (endpoint, credentials) without a recipe no infrastructure is provisioned per-application. We maintain a sub catalog of these shared resource types in the `resource-types-contrib` with their connection contracts, and the environment provisioning process ensures the metadata is injected for agents to use when generating `app.bicep`.

## 2. Recipes

Recipes are how Radius deploys infrastructure behind resource types. Though it is concept of Radius, the implementation uses existing IaC languages — Bicep and Terraform. The proposal here is to leverage reference well-established community maintained modules directly wherever available rather than maintaining custom recipe code. This section covers two things: (1) which module ecosystems we leverage per cloud platform, and (2) Direct Recipe Module Support, the feature that makes referencing these modules easier without wrapper Recipe code.

### Community Module Ecosystems

Radius supports Bicep and Terraform recipe drivers today. Each cloud platform has a dominant IaC tool with an established library of reusable modules that Radius recipes reference directly:

| Cloud / Platform | IaC Tool | Module Library | Registry |
|------------------|----------|----------------|----------|
| Azure | Bicep | [Azure Verified Modules (AVM)](https://aka.ms/avm) | `mcr.microsoft.com/bicep/avm/` |
| AWS | Terraform | [terraform-aws-modules](https://registry.terraform.io/namespaces/terraform-aws-modules) | `registry.terraform.io/terraform-aws-modules/` |
| Kubernetes | Helm | [Bitnami Charts](https://github.com/bitnami/charts) | `oci://registry-1.docker.io/bitnamicharts/` |

Bicep is the Azure-native path through AVM. Terraform provides the broadest AWS and multi-cloud coverage — Stack Overflow 2025 places it at 17.8% adoption across all respondents, well ahead of other infrastructure tools like Ansible (11.7%). CNCF Annual Survey 2025 confirms Helm at 81-87% adoption among Kubernetes organizations, making it the highest-leverage next driver for Radius — it maps directly to how the K8s ecosystem packages and distributes software.

> **Why not CloudFormation for AWS?** Terraform is preferred by multi-cloud companies and [terraform-aws-modules](https://registry.terraform.io/namespaces/terraform-aws-modules) has millions of weekly downloads across 80+ modules. The CloudFormation registry exists but does not contain an authoritative library of reusable infrastructure modules. Modules must be activated per-account/per-region before use, unlike Terraform where a registry URL is referenced directly.

All the module ecosystems use semantic versioning, which means Radius can pin recipe references to exact versions (e.g., `mcr.microsoft.com/bicep/avm/res/db-for-postgre-sql/flexible-server:0.15.3`)and rely on versioning from the module ecosystems.

### Direct Recipe Module Support

Today, using a community module as a Radius Recipe requires a wrapper file that adds a `context` input and a structured `result` output conforming to Radius conventions. This wrapper adds friction, creates maintenance burden to republish to another IaC source and needs constant updates to stay in sync with upstream changes.

Direct Recipe Module Support eliminates the Recipe wrapper for community modules. Platform engineers point `location` at any standard Bicep or Terraform module. Radius handles input resolution (injecting context like resource name and other resource properties like `size` into the module's native parameters via `{{context.*}}` expressions) and output resolution (mapping the module's native outputs to resource properties), all externally.

The Recipe Pack bundles recipe definitions for multiple resource types. It maps each type to a module location, handles parameter injection via `{{context.*}}` expressions, and maps module outputs back to resource properties.

```bicep
// RecipePack resource definition
resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'azure-production'
  properties: {
    recipes: {
      'Radius.Data/postgreSqlDatabases': {
        kind: 'bicep'
        location: 'mcr.microsoft.com/bicep/avm/res/db-for-postgre-sql/flexible-server:0.15.3'
        parameters: {
          name: 'pg-{{context.resource.name}}'
        }
        outputs: {
          host: 'fqdn'
          database: 'name'
        }
      }
    }
  }
}

// Environment references the recipe pack and provides location
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'my-env'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      azure: {
        scope: '/subscriptions/.../resourceGroups/my-rg'
      }
      kubernetes: {
        namespace: 'my-app-ns'
      }
    }
  }
}
```

For the full technical specification, see [Direct Recipe Modules](https://github.com/radius-project/radius/pull/11876).

This feature is critical to scaling the Recipe catalog for the ~27 resource types. It enables us to leverage the rich ecosystem of community modules eliminating the need for maintaining custom wrapper code in the `resource-types-contrib` repository. We maintain the Radius resource type definitions and the mapping configuration in the Recipe Packs in the `resource-types-contrib` repository, making it seamless not only for Application modeling skill but also for Platform engineers who want to use community modules out of the box in the Radius environments.

## Contribution Model and Ownership

The `resource-types-contrib` repository accepts contributions to resource type schemas, module references tested and added to the Recipe Packs. Types graduate through maturity stages **Alpha**, **Beta**, **Stable** with each of them having their own criteria for promotion. Radius Maintainers are responsible to provide clear contribution guidelines, testing workflows to review contributions for quality and consistency, and manage the overall health of the catalog. The contribution process is designed to be inclusive and encourage participation from the community while ensuring that all additions meet the standards required for production use.

## Risks and Dependencies

- **Upstream module quality**: We reference community modules we don't own. If AVM or a Terraform module ships a bad release or drops an output, our recipes break. We pin versions and run weekly CI to catch this early.
- **Helm driver**: Kubernetes recipes are stuck on custom Bicep until we build the Helm driver. That's a bottleneck for K8s coverage.
- **AVM gaps**: Some Tier 1 types don't have AVM modules yet. For those, we fall back to Terraform or custom Bicep on Azure.
- **Schema stability**: Agents depend on resource type schemas. If we break a schema after agents are already using it, generated `app.bicep` files will be wrong. We enforce backward compatibility and have maturity gates to minimize this risk.

## Action Plan

| Priority | Work Item |
|----------|-----------|
| **P0** | Fix deployment engine bugs with AVM modules |
| **P0** | Direct Module Support |
| **P0** | Build Tier 1 Resource type definitions and Recipes |
| **P1** | Test framework for Resource Types and Recipes |
| **P1** | Revamp contrib repository documentation with contribution gates |
| **P1** | Build Tier 2 Resource type definitions and Recipes |
| **P1** | Helm recipe driver |
| **P2** | Recipe packs (dev-local, azure-prod, aws-prod) |
| **P2** | Shared resource types (connection metadata, no recipe) |
| **P3** | CloudFormation driver for native AWS recipe support |
| **P3** | Build Tier 3 Resource type definitions and Recipes |

## Success Metrics

The primary measure is whether the resource type catalog is broad enough for AI agents to generate useful `app.bicep`.

### Resource type and Recipe correctness

- **Benchmark coverage**: We need to benchmark against a representative sample of real-world applications (e.g., top GitHub repos, popular Docker images) to ensure the resource types and recipes cover the dependencies they use. Success means 80%+ of app dependencies map to a defined resource type with a working recipe.
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
3. [Azure Verified Modules](https://aka.ms/avm)
4. [Terraform AWS Modules](https://registry.terraform.io/namespaces/terraform-aws-modules)