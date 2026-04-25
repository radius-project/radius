# Platform Engineering Constitution — Radius

## 1. Overview

This document codifies the conventions for building cloud-native applications
on [Radius](https://radapp.io/). It is intended as a starting point for teams
adopting Radius. Following these conventions produces application definitions
that are valid, portable, and consistent across teams.

Fork this document into your own repository and adapt the marked sections
(regions, tags, registries, naming) to your organization.

## 2. Cloud Providers

### Approved Providers

| Provider                          | Status   | Primary Use Cases                                |
| --------------------------------- | -------- | ------------------------------------------------ |
| Any Kubernetes cluster            | Approved | Target compute for all Radius applications       |
| Container registry (e.g. GHCR, ACR, ECR) | Approved | Container image storage and distribution  |

- Radius is **cloud-agnostic**: any conformant Kubernetes cluster (AKS, EKS,
  GKE, k3d, on-prem) is a valid target.
- Container image references MUST be lowercase.
- Pick a single registry per environment and configure registry credentials
  at the platform layer (see §8) — not per application.

## 3. Compute Platform

### Approved Compute Targets

| Platform                | Status   | Use Case                                    |
| ----------------------- | -------- | ------------------------------------------- |
| Kubernetes (via Radius) | Approved | Primary compute for all containerized apps  |

### Standards

- Workloads are modeled as `Radius.Compute/containers` and deployed by the
  Radius `containers` recipe as a Kubernetes Deployment.
- Recipes deploy into a namespace derived from the Radius group and
  application name — keep group/app names short and DNS-safe.
- All workloads should declare resource `requests` and `limits` on each
  container.
- Use Radius applications and groups (not bare Kubernetes namespaces) to
  isolate apps and environments.

## 4. Infrastructure Policies

### Governance Requirements

- All Radius resources are deployed via IaC (`app.bicep` + `rad deploy`); no
  ad-hoc `kubectl apply` for application resources.
- Application changes go through pull request review.
- Production deployments require at least one approval.
- Resource types are a **closed set** (see §5). Inventing types or
  properties is forbidden.
- Recipe registrations should be pinned to a specific commit SHA, not a
  branch.

## 5. Infrastructure as Code

### Approved Tooling

| Tool      | Status   | Use Case                                            |
| --------- | -------- | --------------------------------------------------- |
| Bicep     | Approved | Radius application definitions (`app.bicep`) and recipe implementations       |
| Terraform | Approved | Recipe implementations |
| `rad` CLI | Approved | Build, deploy, and manage Radius resources          |

### Approved Resource Types

#### Built-in (namespace `Radius.Core`, extension `radius`)

| Need                 | Resource Type                       | API Version          |
| -------------------- | ----------------------------------- | -------------------- |
| Application grouping | `Radius.Core/applications`    | `2025-08-01-preview` |

#### Extensible types (API `2025-08-01-preview`)

| Need                                       | Resource Type                        |
| ------------------------------------------ | ------------------------------------ |
| Container images (build from Dockerfile)   | `Radius.Compute/containerImages`     |
| Containers                                 | `Radius.Compute/containers`          |
| Persistent storage                         | `Radius.Compute/persistentVolumes`   |
| External ingress                           | `Radius.Compute/routes`              |
| MySQL                                      | `Radius.Data/mySqlDatabases`         |
| PostgreSQL                                 | `Radius.Data/postgreSqlDatabases`    |
| Neo4j                                      | `Radius.Data/neo4jDatabases`         |
| Secrets                                    | `Radius.Security/secrets`            |

This is the **complete approved list**. Properties not present in the type's
schema MUST NOT be set.

### Approved Bicep Extensions

| Namespace            | Extension name    |
| -------------------- | ----------------- |
| `Radius.Core`  | `radius`          |
| `Radius.Compute`     | `radiusCompute`   |
| `Radius.Data`        | `radiusData`      |
| `Radius.Security`    | `radiusSecurity`  |

Extensions MUST be declared by namespace (NOT per-type) and in this exact
order, including only those needed:

1. `extension radius`
2. `extension radiusCompute`
3. `extension radiusSecurity`
4. `extension radiusData`

### Bicep File Standards (`app.bicep`)

Every `app.bicep` MUST follow this structure, in this order:

1. **Extensions** — in the order above.
2. **Parameters** —
   - `param environment string` (always; injected by the `rad` CLI)
   - `@secure() param` for any secret value (e.g. database passwords)
   - `param image string` for any container image built from source
3. **Application** — exactly one `Radius.Core/applications@2025-08-01-preview`.
4. **Data / infrastructure** — databases, caches, volumes.
5. **Secrets** — `Radius.Security/secrets` for credentials.
6. **Container images** — `Radius.Compute/containerImages` for builds.
7. **Containers** — `Radius.Compute/containers`, with `connections` to
   images and infrastructure.
8. **Routes** — `Radius.Compute/routes`, only if external ingress is needed.

Structural invariants:

- Exactly **one** `Radius.Core/applications` resource per file.
- One `Radius.Compute/containers` resource per service.
- One `Radius.Data/*` resource per backing data store.
- Container ports use `containerPort` (NOT `port`).
- Validate every file with `rad bicep build app.bicep` before deployment.

## 6. Deployment Standards

### Architecture Patterns

A Radius application is classified into one of these patterns (or a
composition of them) before any Bicep is generated:

- **Single container web app** — one `Radius.Compute/containers`,
  optionally fronted by a `Radius.Compute/routes` for external ingress.
- **Container + datastore** — adds one `Radius.Data/*` and a
  `Radius.Security/secrets` resource holding its credentials.
- **Build-from-source** — adds a `Radius.Compute/containerImages` that
  builds the image from a local `Dockerfile` and is referenced by the
  container.
- **Stateful workload** — adds a `Radius.Compute/persistentVolumes` mounted
  into the container.

Multi-service applications compose these patterns by emitting one
`Radius.Compute/containers` resource per service.

### Deployment Workflow

- Deployments run from CI (e.g. GitHub Actions) using short-lived,
  workflow-scoped credentials for registry authentication. Long-lived
  registry secrets do not live in application repos.
- The deploy command is `rad deploy app.bicep -p image=<image-ref>`. Image
  references and similar parameters are passed via `-p`; secrets are not.

### Naming Conventions

Naming should be **deterministic** so that AI tools and humans converge on
the same output. Adopt a single rule per resource and apply it
mechanically — the example rules below are a recommended starting point:

| Resource              | `name` value (string)                                  |
| --------------------- | ------------------------------------------------------ |
| Application           | Repository name in kebab-case                          |
| Container (single)    | `<app-name>-frontend`                                  |
| Container (per svc)   | Service name in kebab-case                             |
| Container image       | `<app-name>-image`                                     |
| Database              | Engine short name (`mysql`, `postgres`, `neo4j`)       |
| Database secret       | `<app-name>-dbsecret`                                  |
| Route                 | `<app-name>-route`                                     |

Symbolic names (left side of `=`) should mirror the resource name in
camelCase (e.g. `todoApp`, `todoContainer`, `database`, `dbSecret`).

Other fixed values to standardize across the org:

- Port key in `ports` map for HTTP: `web`.
- Container key in the `containers` map: the service short name.
- Image references: lowercase.

### Connections Between Resources

Resources are wired together via the `connections` map on a container.
Connections cause the connected resource's properties to be **automatically
injected** as environment variables in the container.

Rules:

- NEVER duplicate auto-injected env vars with manual `env` entries.
- Only add explicit `env` entries for app-specific variables not covered by
  connection auto-injection.
- A container that uses an image built by `Radius.Compute/containerImages`
  MUST reference the image via `<imageSymbol>.properties.image` AND declare
  a connection to `<imageSymbol>.id`.
- A container that uses a `Radius.Data/*` resource MUST connect to that
  resource's `.id`.

Use stable, predictable connection keys (e.g. `mysqldb`, `postgresdb`,
`neo4jdb`, `containerImage`) so applications can rely on the resulting env
var names.

## 7. Network Architecture

- External ingress is modeled exclusively via `Radius.Compute/routes`. Add
  a route only when external access is required.
- Internal service-to-service communication is established through Radius
  `connections` (which inject the env vars needed for service discovery).
- Restrict public ingress to routes — never expose containers directly.
- The recipe-managed namespace is the network isolation boundary for an
  application's workloads.

## 8. Security & Secrets

### Secret Management

| Tool                          | Status   | Use Case                                                                  |
| ----------------------------- | -------- | ------------------------------------------------------------------------- |
| `Radius.Security/secrets`     | Approved | Application/database credentials referenced by Radius resources           |
| `@secure() param`             | Approved | Sensitive Bicep parameters (e.g., passwords)                              |
| Platform-managed registry creds | Approved | Container registry authentication (configured by platform engineers)    |

### Application Secrets

- Database credentials MUST be modeled as a `Radius.Security/secrets`
  resource and referenced from the data resource via `secretName`.
- Sensitive values (passwords, API keys) MUST come from `@secure() param`
  declarations — never hardcoded in Bicep.
- Applications must never embed secrets in code, config files, or container
  images.
- Rotate secrets on a regular cadence.

### Registry Credentials (Platform-Owned)

- Container registry authentication is a **platform** concern, NOT an
  application concern.
- Credentials are configured once at the platform layer and made available
  to the Radius runtime; application Bicep does not reference them.
- The `registry` property on `Radius.Compute/containerImages` SHOULD be
  omitted in application output — platform-level credentials are used
  instead.
- Do NOT use `Radius.Security/secrets` for registry credentials.

### Platform Engineer vs. Developer Responsibilities

| Responsibility                                                            | Owner             |
| ------------------------------------------------------------------------- | ----------------- |
| Install Radius on the cluster                                             | Platform engineer |
| Configure RBAC for the Radius control plane                               | Platform engineer |
| Configure registry credentials at the platform layer                      | Platform engineer |
| Register resource types and recipes (pinned to commit SHAs)               | Platform engineer |
| Provision per-namespace prerequisites (e.g. image-pull secrets)           | Platform engineer |
| `app.bicep`, `bicepconfig.json`, application source, `Dockerfile`         | Developer         |
| CI workflow that runs `rad deploy`                                        | Developer         |
| Running `rad deploy app.bicep -p image=<image-ref>`                       | Developer         |

## 9. Appendix

### Validation Checklist

Before any `app.bicep` is committed or deployed, verify ALL of the
following:

- [ ] Application resource uses `Radius.Core/applications@2025-08-01-preview`.
- [ ] Every `Radius.*` type is in the approved list (§5) and uses API version `2025-08-01-preview`.
- [ ] Properties on each resource exist in that resource type's schema.
- [ ] Extensions appear in the order: `radius`, `radiusCompute`, `radiusSecurity`, `radiusData`.
- [ ] Names follow the conventions in §6.
- [ ] `param environment string` is declared.
- [ ] `@secure() param` is used for every sensitive value.
- [ ] `param image string` is declared if container images are built from source.
- [ ] Exactly one `Radius.Core/applications` resource.
- [ ] Database resources reference a `Radius.Security/secrets` via `secretName`.
- [ ] `connections` is a top-level property under `properties` (not inside `containers`) and is an object map.
- [ ] Container ports use `containerPort`.
- [ ] Image references are lowercase and supplied via `param`, not hardcoded.
- [ ] `rad bicep build app.bicep` succeeds.

### Change Log

| Date       | Author | Change Description           |
| ---------- | ------ | ---------------------------- |
| 2026-04-20 | —      | Initial constitution         |
