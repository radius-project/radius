# `Radius.Compute/containerImages` resource type

* **Author**: Will Smith (@willdavsmith)

## Overview

`Radius.Compute/containerImages` is a new Radius resource type that
builds a container image from source and pushes it to a registry as
part of an application deployment. Once the image is published, any
`Radius.Compute/containers` resource in the same application can
reference it.

The resource type closes a gap in the Radius application model. Today,
developers must build and push container images out-of-band (locally,
in CI, or by hand) and then point Radius at the resulting image
reference. With `containerImages`, the build is part of the
application graph: Radius knows about the source, the platforms, and
the resulting image, and a deploy that includes a code change
produces a new image as a normal part of reconciliation.

This document is the design for the resource type and its implementation in Radius. 
It covers the user-facing schema, the Terraform recipe that implements
it, and how dynamic-rp executes the build inside a Kubernetes
cluster without privileged workloads or host-side prerequisites.

## Terms and definitions

| Term | Definition |
|---|---|
| **BuildKit** | The container build engine that modern Docker uses internally. Has a maintained "rootless" container image suitable for running inside Kubernetes Pods. |
| **buildkitd** | The BuildKit daemon. Listens on a unix or TCP socket and accepts build requests from clients (the Docker CLI, `buildctl`, etc.). |
| **buildctl** | The BuildKit CLI client. Ships in the same upstream image as buildkitd. Speaks BuildKit's gRPC protocol over either a unix or TCP endpoint and is the client the recipe shells out to. |
| **Rootless mode** | A BuildKit operating mode where the daemon runs as a non-root user (UID 1000) and uses Linux user namespaces instead of host capabilities to isolate build steps. Avoids `privileged: true` and `CAP_SYS_ADMIN`. |
| **Pod Security Admission (PSA)** | Kubernetes' built-in admission controller for Pod security profiles. Defines three profiles: `privileged`, `baseline`, `restricted`. |
| **User namespaces (Kubernetes)** | The `hostUsers: false` Pod field, backed by the `UserNamespacesSupport` feature gate. Stable in Kubernetes 1.33; beta on-by-default in 1.30. Lets a Pod use Linux user namespaces without relaxing seccomp/AppArmor. |
| **Cross-compile** | A build strategy where the toolchain (e.g. `GOARCH=arm64 go build`) emits foreign-architecture artifacts from a native-architecture builder. The standard mechanism in Dockerfiles is `FROM --platform=$BUILDPLATFORM` plus `TARGETARCH`. |
| **Native fan-out** | A multi-architecture build strategy where one builder Pod per architecture runs natively on a node of that architecture. Out of scope for this iteration. |

## Objectives

### Goals

* Provide a `Radius.Compute/containerImages` resource type that
  developers can declare in Bicep and that produces a published
  container image.
* Build images on **any Kubernetes cluster** Radius supports —
  managed (EKS / AKS / GKE), self-hosted, and local (k3d / kind).
* Avoid host privilege: no host volume mounts, no `privileged: true`,
  no added Linux capabilities, no host networking, no host kernel
  preparation.
* Support multi-architecture builds in the common case
  (cross-compile-friendly Dockerfiles).
* Default to a Pod security posture compatible with PSA `baseline`
  out of the box, with PSA `restricted` available as an opt-in for
  operators on Kubernetes 1.30+ who want the stricter posture.

### Non-goals

* A long-running, multi-tenant build service exposed as a separate
  Radius component.
* A container registry. Recipes push to whatever registry the
  developer or operator configures.
* Generating Dockerfiles, recommending base images, or any
  language-specific build tooling.
* Multi-architecture builds via native node-pool fan-out. (See
  [Appendix](#appendix-multi-architecture-node-pools).)
* Multi-architecture builds via QEMU/binfmt emulation. (See
  [Alternatives](#alternatives-considered).)
* Bicep recipe implementation. (See https://github.com/radius-project/resource-types-contrib/issues/131).

### User scenarios

#### User story 1 — Multi-architecture image on a single-arch cluster

A developer working on an amd64-only AKS cluster needs both
`linux/amd64` and `linux/arm64` images for downstream environments.
They don't need to specify special configuration, since Radius 
publishes the multi-arch build by default. Their Dockerfile uses
`FROM --platform=$BUILDPLATFORM` and `TARGETARCH`, so both
architectures build cross-compile on the amd64 builder and a
manifest list is pushed.

#### User story 2 — Developer iterates on an image locally

A developer has a service in `./frontend` of their working tree.
They add a `containerImages` resource to their Bicep file, set
`build.context: './frontend'`, and reference the resulting image
from a `containers` resource. `rad deploy` tarballs the local
directory, uploads it to dynamic-rp, builds via the in-cluster
BuildKit, pushes to the recipe-configured registry, and rolls the
container. Inner-loop iteration uses no out-of-band `docker build`
or `docker push`.

#### User story 3 — Developer builds from a git URL

A developer has already pushed their code to a git repository and
wants to build directly from there instead of uploading their
working tree. The same Bicep, but
`build.context: 'git::https://github.com/alice/myapp.git//frontend'`.
BuildKit clones the repo inside the cluster on each deployment. 
All git url constructs (refs, tags, sha, branch) are supported.

#### User story 4 — Operator installs Radius on a regulated cluster

An operator runs Radius on a cluster that enforces PSA `restricted`
cluster-wide and runs Kubernetes 1.30+ with the
`UserNamespacesSupport` feature gate. They install Radius with
`--set dynamicrp.buildkit.psaMode=restricted`; the buildkitd sidecar
runs in `restricted`-compatible mode (Kubernetes user namespaces)
without policy exceptions.

## User Experience

### Sample input

```bicep
extension radius
extension containerImages
// Disambiguate from the radius extension's Radius.Compute/containers
// (which lacks imagePullSecrets).
extension containers as ctnrs

param environment string
param imageTag string
param buildContext string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'myapp'
  properties: { environment: environment }
}

resource frontendImage 'Radius.Compute/containerImages@2025-08-01-preview' = {
  name: 'todolist-app'
  properties: {
    environment: environment
    application: app.id
    tag:         imageTag
    build: { context: buildContext }
  }
}

resource frontend 'ctnrs:Radius.Compute/containers@2025-08-01-preview' = {
  name: 'frontend'
  properties: {
    environment: environment
    application: app.id
    imagePullSecrets: [frontendImage.properties.imagePullSecretName]
    containers: {
      app: {
        image: frontendImage.properties.image
        ports: { web: { containerPort: 3000 } }
      }
    }
  }
}
```

The developer never declares a registry secret, references one by
name, or supplies registry credentials as `@secure()` parameters.
Registry authentication is wired in once per environment by the
platform engineer (see [Recipe registration](#recipe-registration));
the recipe materializes a per-resource pull Secret and surfaces its
name as `imagePullSecretName` for `Radius.Compute/containers` to
consume.

The developer never writes the registry, the repository path, or
the tag. The final image reference is composed by the recipe:

```
<registry>/<resource-name>:<tag>
ghcr.io/mycompany/todolist-app:sha256-d4f2…
└──────┬────────┘ └─────┬────┘ └─────┬─────┘
   from registry    from the       content-addressable
   recipe param     resource name  tag (default)
```

Tags default to a content-addressable digest (see [Tag strategy](#tag-strategy)).
Developers can override per-resource by setting `properties.tag`.
When `build.context` is a remote git URL the recipe cannot cheaply
hash the tree, so `properties.tag` is required and a
`terraform_data "validate_git_tag"` precondition fails the deploy
otherwise.

### Recipe registration

The platform engineer deploys a single `platform.bicep` that:

1. Declares a `Radius.Core/recipePacks` registering the
   `Radius.Compute/containerImages` recipe with three parameters —
   `registry` (push target), `registrySecretName`, and
   `registrySecretNamespace` (location of the kube Secret holding
   `username` / `password`). Same pack typically registers
   `Radius.Compute/containers` and `Radius.Security/secrets`.
2. Declares a `Radius.Core/environments` that references the pack.
3. Declares a `Radius.Core/applications` named `platform` that holds
   PE-owned resources.
4. Declares a `Radius.Security/secrets` of `kind: generic` with
   `username` / `password` data keys in the `platform` app. Its
   recipe materializes a Kubernetes Secret of the same name in the
   env's namespace, which is what the containerImages recipe reads
   at execution time.

```bicep
extension radius

param registryPath string
param registryUsername string
@secure()
param registryPassword string
param containerImagesTemplatePath string
param containersTemplatePath string
param envNamespace string = 'default'

resource recipes 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'default-recipes'
  location: 'global'
  properties: {
    recipes: {
      'Radius.Security/secrets': {
        recipeKind: 'terraform'
        recipeLocation: 'git::https://github.com/radius-project/resource-types-contrib.git//Security/secrets/recipes/kubernetes/terraform'
      }
      'Radius.Compute/containerImages': {
        recipeKind: 'terraform'
        recipeLocation: containerImagesTemplatePath
        parameters: {
          registry:                registryPath
          registrySecretName:      'ghcr-creds'
          registrySecretNamespace: envNamespace
        }
      }
      'Radius.Compute/containers': {
        recipeKind: 'terraform'
        recipeLocation: containersTemplatePath
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'default'
  location: 'global'
  properties: {
    providers:    { kubernetes: { namespace: envNamespace } }
    recipePacks:  [ recipes.id ]
  }
}

resource platform 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'platform'
  location: 'global'
  properties: { environment: env.id }
}

resource ghcrCreds 'Radius.Security/secrets@2025-08-01-preview' = {
  name: 'ghcr-creds'
  properties: {
    environment: env.id
    application: platform.id
    kind: 'generic'
    data: {
      username: { value: registryUsername }
      password: { value: registryPassword }
    }
  }
}
```

Key points:

* The **`registry`** recipe parameter is the full prefix
  (registry host plus optional namespace/org); the resource
  name supplies the final path segment. Different environments
  point their recipePack at different registries
  (dev → `ghcr.io/alice`, prod → `myorg.azurecr.io/prod`)
  without any change to the developer's Bicep.
* **Registry credentials** live in a `Radius.Security/secrets` of
  `kind: generic` deployed by the PE into a dedicated `platform`
  application. Its recipe writes a Kubernetes Secret in the env's
  namespace. The containerImages recipe reads it at execution time
  via the `kubernetes_secret` data source named by the
  `registrySecretName` / `registrySecretNamespace` recipe
  parameters. Credentials are not in developer Bicep, not in the
  recipe pack, and not in any chart-level Secret mount.
* The recipePack is the single declarative artifact: deploying
  `platform.bicep` registers all three recipes; no imperative
  `rad recipe register` is needed for them.
* Developers do not see or manage the registry host, the secret,
  the pull Secret, or the recipe registration.

### Bring-your-own published image

If a user already builds and publishes their image out-of-band
(local `docker build && docker push`, CI pipeline, GitOps), they
do not need `Radius.Compute/containerImages` at all. They simply
reference the published image directly from a
`Radius.Compute/containers` resource exactly as they do today.
This pre-existing workflow is unchanged and remains supported;
`containerImages` is purely additive for users who want the build
to be part of the application graph.

### Sample output

`rad deploy` reports build progress through the recipe execution log
and produces an image reference at `properties.image`. Downstream
resources that consume that output redeploy with the new digest on
the next reconciliation.

## Design

### High-level design

A `Radius.Compute/containerImages` resource is reconciled by
dynamic-rp like any other recipe-backed resource type. The recipe is
written in Terraform.

> Note:  A core design principle in Radius is that adding a new resource
> type should not require special-casing inside dynamic-rp — recipes
> are the extensibility point. This resource type is an exception. 
> It depends on and calls APIs in dynamic-rp (buildctl CLI).

The following are the high-level components of this design:

1. **Resource type schema** (`containerImages.yaml`):
   defines the user-facing API.
2. **Terraform recipe** (`recipes/kubernetes/terraform/`):
   takes the resource's properties and invokes `buildctl` against
   the local BuildKit endpoint via a `local-exec` provisioner to
   build and push.
3. **dynamic-rp Helm chart** (`deploy/Chart`):
   adds the buildkitd sidecar, the `buildctl-init` init container,
   and the env wiring so the recipe has a client and an endpoint
   to talk to.

### Architecture diagram

```
┌─────────────────────────── dynamic-rp Pod ───────────────────────────┐
│                                                                      │
│  ┌────────────────────────┐         ┌──────────────────────────┐    │
│  │  dynamic-rp container  │         │  buildkitd container     │    │
│  │                        │         │  (moby/buildkit:rootless)│    │
│  │  ┌──────────────────┐  │         │                          │    │
│  │  │ recipe execution │  │         │  tcp://127.0.0.1:1234    │    │
│  │  │ (Terraform +     │──┼────────▶│  (Pod loopback)          │    │
│  │  │  local-exec      │  │  gRPC   │                          │    │
│  │  │  → buildctl)     │  │         │                          │    │
│  │  └──────────────────┘  │         └────────────┬─────────────┘    │
│  │           ▲            │                      │                  │
│  │           │ buildctl on PATH                  │                  │
│  └───────────┼────────────┘                      │                  │
│              │                                   │                  │
│   ┌──────────┴──────────┐  emptyDir              │                  │
│   │ buildctl-init       │  /opt/buildctl/bin     │                  │
│   │ (initContainer,     │  (buildctl binary)     │                  │
│   │  copies buildctl    │                        │                  │
│   │  from BuildKit img) │                        │                  │
│   └─────────────────────┘                        │                  │
│                                                  │                  │
└──────────────────────────────────────────────────┼──────────────────┘
                                                   │
                                                   │ HTTPS push
                                                   ▼
                                       ┌────────────────────────┐
                                       │  user's container      │
                                       │  registry              │
                                       └────────────────────────┘
```

### Detailed design

#### Resource type schema

The resource type is `Radius.Compute/containerImages`, defined in
`resource-types-contrib` under `Compute/containerImages/containerImages.yaml`.

Properties:

| Property | Type | Required | Description |
|---|---|---|---|
| `environment` | string | no | The Radius Environment ID. Optional so a single built image can be shared across environments. |
| `application` | string | no | The Radius Application ID. Optional so a single built image can be shared across applications. |
| `tag` | string | no | Tag for the produced image. Defaults to a content-addressable digest computed from the build inputs (see [Tag strategy](#tag-strategy)). Required when `build.context` is a git URL. |
| `build.context` | string | yes | Source location. Either a git URL (`git::https://…`) or — for local development workflows — a path that the rad CLI uploads as a tarball. See [Local development workflow](#local-development-workflow). |
| `build.dockerfile` | string | no | Path to the Dockerfile relative to the context. Defaults to `Dockerfile`. |
| `build.platforms` | string[] | no | Target platforms (e.g. `["linux/amd64", "linux/arm64"]`). When omitted, defaults to `["linux/amd64", "linux/arm64"]`. |

The resource **name** (e.g. `todolist-app`) is what the developer
writes in `resource <name> 'Radius.Compute/containerImages@…'`, and
becomes the final path segment of the image reference.

##### Outputs

| Output | Description |
|---|---|
| `properties.image` | The full resolved image reference, e.g. `ghcr.io/mycompany/todolist-app:sha256-d4f2…`. Downstream `Radius.Compute/containers` resources reference this so they pick up new digests automatically. |
| `properties.imagePullSecretName` | The name of the per-resource `kubernetes.io/dockerconfigjson` Secret the recipe materializes in the application's namespace. Empty when the recipe is registered without `registrySecretName` / `registrySecretNamespace` (public-registry path). |

##### Tag strategy

The default tag is a **content-addressable hash** of the build
inputs: a SHA over the build context contents, the Dockerfile, and
the requested platforms. Two reasons:

1. **Correct reconciliation.** Downstream `containers` resources
   reference `frontendImage.properties.image`. If the tag is
   something stable like `latest`, a code change produces a new
   image at the registry but the `containers` resource sees no
   property change and Kubernetes does not roll the Deployment.
   With a content-hash tag, every code change produces a new
   `properties.image` value, downstream sees a real change, and
   reconciliation does the right thing.
2. **Immutability.** Pinned tags can't be overwritten by accident
   from another developer pushing to the same name. Useful for
   audit and rollback.

Developers who need explicit tags (semver releases, git SHAs) set
`properties.tag` directly and accept responsibility for picking
unique values.

#### Terraform recipe

The recipe lives in `Compute/containerImages/recipes/kubernetes/terraform/`
and is intentionally small. Its contract:

* **Build driver**: `buildctl` (the BuildKit CLI), invoked from a
  `terraform_data` resource with a `local-exec` provisioner. The
  recipe shells out to `buildctl` rather than depending on a
  Terraform provider so it uses BuildKit's own client and stays
  small.
* **Endpoint**: configured via the `BUILDKIT_HOST` environment
  variable, which dynamic-rp sets to `tcp://127.0.0.1:1234` (Pod
  loopback to the sidecar) for recipe execution. The recipe itself
  does not encode the endpoint.
* **Authentication**: the recipe takes two PE-provided parameters,
  `registrySecretName` and `registrySecretNamespace`, identifying
  a Kubernetes Secret with `username` and `password` data keys (the
  Secret the PE-owned `Radius.Security/secrets` resource
  materializes). The recipe reads it via the
  [`kubernetes_secret`](https://registry.terraform.io/providers/hashicorp/kubernetes/latest/docs/data-sources/secret)
  data source, **base64-decodes the data values** (the data source
  returns base64 strings, unlike the resource form), composes a
  Docker `config.json`, writes it via `local_sensitive_file`, and
  exports `DOCKER_CONFIG` so `buildctl` picks it up. Both parameters
  default to empty strings; when both are empty the recipe skips the
  auth path entirely (public-registry / local-kind-registry flow).
* **Pull Secret materialization**: after a successful push the
  recipe creates a per-resource `kubernetes.io/dockerconfigjson`
  Secret named `<resource>-pull` in the application's namespace
  (via the same registry credentials) and surfaces its name as the
  `imagePullSecretName` output. `Radius.Compute/containers`
  references that output so kubelet can pull the produced image
  without any out-of-band wiring.
* **Input validation**: every value interpolated into the
  `buildctl` command line is gated by a `terraform_data
  "validate_inputs"` resource with `precondition` blocks (registry,
  resource name, tag, dockerfile path, build context, platforms)
  matched against tight regexes. The build resource declares
  `depends_on = [validate_inputs]` so a bad input fails before any
  shell invocation. This compensates for the fact that
  `local-exec` lacks the structured-parameter contract a Terraform
  provider gives for free.

  A separate `terraform_data "validate_git_tag"` precondition
  rejects git-context resources that omit `properties.tag`. A
  content-hash tag default cannot apply to remote git contexts
  because the recipe has no inexpensive way to hash a remote tree
  before invoking BuildKit; the recipe fails fast with a clear
  message rather than silently push a non-content-addressable image
  on every reconciliation.
* **Recipe parameters**: `registry` (e.g. `ghcr.io/mycompany`),
  `registrySecretName`, and `registrySecretNamespace`. The BuildKit
  endpoint is wired in by dynamic-rp via `BUILDKIT_HOST` and is not
  a recipe parameter.
* **Destroy semantics**: `terraform destroy` removes the Terraform
  state for the resource but does **not** delete the pushed image
  from the registry. Registry retention is an operator concern
  (registry GC policies, immutable-tag flags, etc.). Documented
  explicitly in the recipe README.

Sketch of the resources the recipe declares:

```hcl
locals {
  use_auth = var.registrySecretName != "" && var.registrySecretNamespace != ""
}

data "kubernetes_secret" "registry_creds" {
  count = local.use_auth ? 1 : 0
  metadata {
    name      = var.registrySecretName
    namespace = var.registrySecretNamespace
  }
}

locals {
  # kubernetes_secret data source returns .data as base64-encoded
  # strings; decode once before composing the Docker auth header.
  registry_username = local.use_auth ? base64decode(data.kubernetes_secret.registry_creds[0].data["username"]) : ""
  registry_password = local.use_auth ? base64decode(data.kubernetes_secret.registry_creds[0].data["password"]) : ""

  docker_config_json = local.use_auth ? jsonencode({
    auths = {
      (local.registry_host) = {
        auth = base64encode("${local.registry_username}:${local.registry_password}")
      }
    }
  }) : ""

  resource_name = var.context.resource.name
  registry      = var.registry
  context_sha   = sha256(...)  # over context + dockerfile + platforms
  resolved_tag  = coalesce(
    try(var.context.resource.properties.tag, null),
    "sha256-${substr(local.context_sha, 0, 16)}",
  )
  image_ref     = "${local.registry}/${local.resource_name}:${local.resolved_tag}"
  platforms_csv = join(",", local.platforms)
}

resource "local_sensitive_file" "docker_config" {
  count           = local.use_auth ? 1 : 0
  filename        = "${path.module}/.docker-${local.resource_name}/config.json"
  content         = local.docker_config_json
  file_permission = "0600"
}

# Validate every value that will be interpolated into the buildctl
# command line. The build resource depends on this so bad inputs
# fail before any shell invocation.
resource "terraform_data" "validate_inputs" {
  lifecycle {
    precondition { condition = can(regex("^[a-z0-9./:_-]+$", local.registry))      error_message = "..." }
    precondition { condition = can(regex("^[a-z0-9-]+$",    local.resource_name))  error_message = "..." }
    precondition { condition = can(regex("^[A-Za-z0-9._-]+$", local.resolved_tag)) error_message = "..." }
    # ...dockerfile, build_context, platforms
  }
}

resource "terraform_data" "build_push" {
  triggers_replace = { src_sha = local.context_sha }
  depends_on       = [terraform_data.validate_inputs, local_sensitive_file.docker_config]

  provisioner "local-exec" {
    environment = {
      DOCKER_CONFIG = local.use_auth ? "${path.module}/.docker-${local.resource_name}" : ""
    }
    command = <<-EOT
      buildctl build \
        --frontend=dockerfile.v0 \
        --opt context=${local.build_context} \
        --opt filename=${local.dockerfile} \
        --opt platform=${local.platforms_csv} \
        --output type=image,name=${local.image_ref},push=true
    EOT
  }
}

resource "kubernetes_secret" "pull" {
  count = local.use_auth ? 1 : 0
  metadata {
    name      = "${local.resource_name}-pull"
    namespace = var.context.runtime.kubernetes.namespace
  }
  type = "kubernetes.io/dockerconfigjson"
  data = { ".dockerconfigjson" = local.docker_config_json }
}

output "properties" {
  value = {
    image                = local.image_ref
    imagePullSecretName  = local.use_auth ? "${local.resource_name}-pull" : ""
  }
}
```

Multi-arch is handled by passing multiple platforms to
`--opt platform=`; BuildKit produces a manifest list and pushes it.
No buildx-builder resource is required because the recipe always
talks to a single rootless BuildKit endpoint and uses cross-compile
for foreign architectures.

#### dynamic-rp Helm chart changes

The chart change has four parts: adding the sidecar, dropping a
`buildctl` binary onto the recipe runner's PATH, wiring the
endpoint, and choosing a Pod security profile.

**1. Sidecar container.** Add a second container to the dynamic-rp
Deployment, using the upstream
`moby/buildkit:<pinned-version>-rootless` image, with args
`--addr tcp://0.0.0.0:1234` so it listens on Pod loopback. The
sidecar's liveness/readiness probes use `buildctl debug workers`,
matching upstream's recommended manifest.

**2. `buildctl-init` init container.** A short init container
built from the same `moby/buildkit:<pinned-version>-rootless`
image copies `/usr/bin/buildctl` into an `emptyDir` that is
mounted into the dynamic-rp container at a fixed path on `PATH`.
This is the only thing shared between the two containers: no
build socket and no BuildKit state directory, since the recipe
talks to buildkitd over Pod loopback TCP.

**3. Endpoint wiring.** dynamic-rp gets two env vars:
`BUILDKIT_HOST=tcp://127.0.0.1:1234` and a `PATH` that includes
the `buildctl-init` emptyDir. No Docker `config.json` is mounted
at chart level; registry credentials reach the recipe through the
`registrySecretName` / `registrySecretNamespace` recipe parameters
the PE wires in via `Radius.Core/recipePacks` (see
[Recipe registration](#recipe-registration)).

The sidecar is enabled by default. Operators who never use
`Radius.Compute/containerImages` can opt out by setting
`dynamicrp.buildkit.enabled=false` at install time, in which case
the sidecar, the `buildctl-init` init container, and the
`BUILDKIT_HOST` env var are all omitted and any attempt to deploy
a `containerImages` resource fails fast with a clear error.
Default-on matches the principle that core resource types should
work out of the box; the opt-out exists for operators who care
about the ~50 MiB idle footprint.

**4. Pod security profile.** Selected by the Helm value
`dynamicrp.buildkit.psaMode`, with two settings sharing the same
image and endpoint:

| Mode | Pod / sidecar security controls | When to use |
|---|---|---|
| **`baseline`** (default) | Sidecar sets `seccompProfile: Unconfined`, `appArmorProfile: Unconfined`, args `--oci-worker-no-process-sandbox`. Compatible with PSA `baseline`. | Default. Works on every supported Kubernetes version, including older clusters and local dev environments (kind, k3d, Docker Desktop) without extra configuration. |
| **`restricted`** | `pod.spec.hostUsers: false`, pod-level `securityContext.fsGroup: 65532` (so the dynamic-rp container can read the `buildctl` binary the init container drops into the shared emptyDir). Sidecar has no `Unconfined` profiles, no `--oci-worker-no-process-sandbox`. Compatible with PSA `restricted`. | Opt-in for operators who enforce PSA `restricted` cluster-wide and run Kubernetes user namespaces (stable in 1.33+, beta on-by-default in 1.30+). |

Neither mode uses `privileged: true`, mounts host paths, or
requires added Linux capabilities. Defaulting to `baseline` keeps
the install command a one-liner on every cluster Radius supports;
operators who need the stricter posture explicitly opt in.

The chart includes a Helm `NOTES.txt` preflight that surfaces a
clear message ("Kubernetes ≥ 1.30 with UserNamespacesSupport
required; reinstall without `--set dynamicrp.buildkit.psaMode=restricted`")
if `restricted` is selected on an incompatible cluster.

#### Implementation choice: recipe vs. built-in provider

The design above implements `Radius.Compute/containerImages` as a
Terraform recipe. There is a viable alternative: implement the
build path as a **built-in resource provider** inside dynamic-rp,
calling BuildKit's gRPC API directly from Go.

| | Terraform recipe (proposed) | Built-in provider (alternative) |
|---|---|---|
| Build invocation | `local-exec` → `buildctl` → BuildKit gRPC | Go BuildKit client → BuildKit gRPC |
| Auth wiring | PE-deployed `Radius.Security/secrets` of `kind: generic`; recipe resolves it via the `kubernetes_secret` data source using the `registrySecretName` / `registrySecretNamespace` recipe parameters | Same `Radius.Security/secrets`, resolved directly by dynamic-rp |
| Customization | Operators can swap in their own Terraform recipe (different tag scheme, additional provenance, signing, etc.) | Behavior is fixed by dynamic-rp; customization requires a code change |
| Failure modes | Inherits everything Terraform brings (state, lockfiles, provider version drift) | Fewer moving parts, no Terraform state for an action that has no resources to track |
| Consistency with rest of Radius | Matches every other resource type | This type is already a special case (BuildKit sidecar); a built-in provider would not break a pattern this resource type doesn't fit |

**Direction: ship as a Terraform recipe.** The recipe preserves
the customization story (signing, provenance, alternative tag
schemes) and matches every other resource type, at the cost of
inheriting Terraform state for a resource that does not really
have any. The special-casing that breaks the "no special cases"
property is contained in dynamic-rp's chart and env (the BuildKit
sidecar, `buildctl-init`, `BUILDKIT_HOST`), not in the recipe
contract itself. The user-facing schema and the BuildKit sidecar
are the same either way; only the box in the architecture diagram
labelled "recipe execution" changes.

#### Multi-architecture builds

The design assumes the cluster is **single-architecture**. A single
node pool of one architecture is the common case for managed
Kubernetes today and for local development. Multi-architecture
images are produced by **cross-compilation**: the Dockerfile uses
the standard `BUILDPLATFORM` / `TARGETPLATFORM` build args, and all
build steps run on the sidecar's native architecture while the
toolchain emits foreign-arch artifacts.

```dockerfile
# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY . .
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/app .

FROM alpine
COPY --from=build /out/app /app
ENTRYPOINT ["/app"]
```

This works for the language stacks Radius users typically reach for
first (Go, Rust, JVM, most Node and Python apps that just package
artifacts).

CPU emulation (QEMU/binfmt) is deliberately not used. It is slow
(5–20× native), unreliable for many Dockerfiles, and requires
either a privileged host-prep step or a privileged init container
to register `binfmt_misc` on the host kernel — both of which this
design is trying to eliminate. Operators who need it for legacy
Dockerfiles can install it themselves on their nodes, and BuildKit
will use it transparently — but Radius does not ship it.

##### Behavior when the Dockerfile isn't cross-compile-friendly

This is one of the bigger UX questions in the design. There are
three plausible behaviors when a developer requests
`platforms: [linux/amd64, linux/arm64]` and their Dockerfile has a
`RUN` step that won't cross-compile (for example, an `apt-get
install` whose post-install scripts execute target-arch binaries):

| Behavior | Outcome |
|---|---|
| **A. Fail loudly (proposed)** | BuildKit returns `exec format error` for the foreign architecture. The recipe surfaces a wrapped error: *"Multi-arch build requested but the Dockerfile isn't cross-compile-friendly. Either rewrite using `FROM --platform=$BUILDPLATFORM` and `TARGETARCH`, or request a single platform."* The recipe does not push anything. |
| B. Silent single-arch fallback | The recipe drops architectures that fail and pushes a manifest list containing only the architectures that succeeded (or just the native arch). |
| C. Silent native-only build | The recipe ignores `platforms` whenever it can't satisfy them and just builds the sidecar's native architecture. |

**The proposed behavior is A — fail loudly.** B and C produce a
class of bug that's worse than failing: deploys appear to succeed,
the developer's `containers` resource pulls the image successfully
on their amd64 laptop or their amd64 CI worker, and only at runtime
on an arm64 production node does the workload crash with `exec
format error`. By that point the build trail is gone and the cause
is opaque. Failing at build time, where the diagnostics are
immediate and the remediation is one Dockerfile edit away, is
strictly better.

The cost of A is that some users who request multi-arch
*incidentally* (because the recipe template they copied had it) will
hit the error and need to either remove the second platform or fix
the Dockerfile. The error message points them at both.

A future iteration could add native fan-out (see the appendix) for
the case where the Dockerfile genuinely can't be cross-compiled.
That's an additive change to behavior A, not a replacement for it.

#### Local development workflow

There are three workflows the design supports, each
suited to a different inner-loop tempo:

##### Option 1 — Git context (default for CI / GitOps)

The developer sets `build.context` to a `git::https://…` URL.
BuildKit clones the repo inside the cluster and builds. This is
the right answer for CI pipelines and GitOps reconciliation
(Flux, Argo, etc.) where the source of truth is the git
repository. It is **not** the right answer for an inner dev loop
because the developer has to push every change.

##### Option 2 — `rad` CLI tarball upload (proposed for inner-loop dev)

The developer sets `build.context` to a local path
(`./frontend`). The `rad deploy` CLI detects local-path contexts,
tars the directory (honoring `.dockerignore`), and uploads the
tarball to dynamic-rp as part of the deployment payload.
dynamic-rp serves it to BuildKit as the build context. Each
`rad deploy` produces a fresh build with the developer's working
copy.

To enable this, rad CLI POSTs the tarball to a
new dynamic-rp endpoint, dynamic-rp stages it in an emptyDir,
the recipe reads from there. Symmetric with how recipes
already receive parameters; one new endpoint.

##### Option 3 — Build-locally

The developer uses their own `docker build` / `docker push`
out-of-band and references the resulting image directly from a
`Radius.Compute/containers` resource — not via
`Radius.Compute/containerImages` at all. This is the existing
pre-`containerImages` workflow; it remains supported because not
every team wants to put their build through the cluster's
BuildKit, especially when iterating very quickly with local
caches warmed.

This option is documented for completeness but not optimized for
in this design. Developers who want it just don't use the
`containerImages` resource type.

##### Local cluster considerations (k3d / kind / Docker Desktop)

The proposed design works on local clusters without any host
preparation: no shared Docker socket, no extra mounts, no
binfmt registration. The buildkitd sidecar runs inside the local
cluster like any other container.

* **Image visibility.** Local clusters generally cannot pull from
  arbitrary public/private registries without credentials. The
  recommended pattern is to register a recipe that targets a
  local registry — e.g. k3d's built-in registry add-on
  (`k3d cluster create --registry-create radius-registry`)
  exposes a registry the cluster can pull from. The recipe
  `registry` parameter points at it.
* **Multi-arch.** Local clusters are single-arch (the host
  architecture). Cross-compile multi-arch works the same way it
  does on managed Kubernetes; native fan-out doesn't (and isn't
  in v1 anyway).
* **Performance.** BuildKit's first-build cold cache is the same
  as it would be on any cluster. Subsequent builds reuse the
  sidecar's `emptyDir` cache, which survives recipe re-runs but
  not Pod restarts.

#### Why BuildKit (and not Docker, Buildah, Kaniko, or others)

* BuildKit has a maintained rootless container image upstream, with
  documented Kubernetes manifests for both `baseline`-style and
  user-namespace deployment.
* BuildKit ships `buildctl` in the same image as `buildkitd`, so
  the recipe gets a first-party client without pulling in a
  separate Docker CLI or a Terraform provider that wraps it.
* BuildKit handles cross-compile multi-architecture builds as a
  first-class feature.
* BuildKit is the engine that modern Docker itself uses internally,
  which keeps the recipe behavior consistent with what developers
  see locally with `docker buildx`.

### Advantages

* `Radius.Compute/containerImages` works on production clusters,
  not just local-dev.
* No host-side prerequisites for users or operators in the common
  case: no host volume mounts, no host kernel preparation, no
  privileged Pods.
* Default install is compatible with PSA `baseline` on every
  supported Kubernetes version. Operators who enforce PSA
  `restricted` cluster-wide can opt in to the stricter sidecar
  profile on Kubernetes 1.30+ via
  `--set dynamicrp.buildkit.psaMode=restricted`.
* Single image for both PSA modes; differences are limited to the
  Pod's `securityContext` block.

### Disadvantages

* **Two PSA profiles to support.** `baseline` (default) works on
  every supported Kubernetes version. Operators who want the
  `restricted` posture on Kubernetes 1.30+ opt in via
  `dynamicrp.buildkit.psaMode=restricted`. The chart fails fast
  with a clear message if `restricted` is selected on an
  incompatible cluster.
* **Idle resource cost.** The buildkitd sidecar adds ~50 MiB
  resident memory to every dynamic-rp Pod. It is on by default;
  operators who never use `containerImages` can opt out via
  `dynamicrp.buildkit.enabled=false`.
* **Dockerfiles must be cross-compile-friendly for multi-arch.**
  Multi-architecture builds rely on `BUILDPLATFORM` /
  `TARGETPLATFORM`. Dockerfiles that execute target-arch binaries
  during the build won't work multi-arch under this design and will
  fail at build time with a clear error.
* **Credential bootstrap.** The PE deploys a single
  `Radius.Security/secrets` resource into a dedicated `platform`
  application and points the recipe at the materialized Kubernetes
  Secret via the `registrySecretName` / `registrySecretNamespace`
  recipe parameters. Operator UX for rotating PATs, scoping
  per-application vs. per-environment secrets, etc. is a separate
  workstream.
* **Local-context upload size.** The CLI tarball-upload path
  (Option 2 in [Local development workflow](#local-development-workflow))
  scales poorly for very large directories. `.dockerignore` and a
  reasonable size cap mitigate but don't eliminate this. Multi-GiB
  contexts probably need git.

### Implementation details

| Component | Repo | Change |
|---|---|---|
| Resource type schema | `radius-project/resource-types-contrib` | Add `Compute/containerImages/containerImages.yaml`. Required: `build`. Optional: `environment`, `application`, `tag`. Outputs: `image`, `imagePullSecretName`. Extend `Compute/containers/containers.yaml` with an `imagePullSecrets: array<string>` property. |
| Terraform recipe | `radius-project/resource-types-contrib` | Add `Compute/containerImages/recipes/kubernetes/terraform/{main.tf,var.tf}`. Recipe shells out to `buildctl` via a `terraform_data` + `local-exec` provisioner targeting `BUILDKIT_HOST`. Inputs are validated by `terraform_data "validate_inputs"` and `terraform_data "validate_git_tag"` preconditions; the build resource `depends_on` both. Reads the registry Kubernetes Secret via the `kubernetes_secret` data source named by the `registrySecretName` / `registrySecretNamespace` recipe parameters, `base64decode`s the `username` / `password` values, renders `config.json` via `local_sensitive_file`, and exports `DOCKER_CONFIG`. After push, creates a per-resource `kubernetes.io/dockerconfigjson` Secret in the application namespace and surfaces it as `imagePullSecretName`. Composes image ref from `registry`, resource name, and content-hash tag (or explicit `properties.tag` when supplied). The `Compute/containers` kubernetes recipe threads `imagePullSecrets` into the Deployment's `template.spec.image_pull_secrets`. |
| Recipe pack | `radius-project/resource-types-contrib` (samples / docs) | Document the `Radius.Core/recipePacks` registration flow: pack registers the `containerImages` recipe with `registry`, `registrySecretName`, and `registrySecretNamespace` parameters; PE separately deploys a `Radius.Security/secrets` of `kind: generic` into a `platform` application that materializes the referenced Kubernetes Secret in the env's namespace. |
| dynamic-rp Helm chart | `radius-project/radius` (`deploy/Chart`) | Add `buildkitd` sidecar (default-on, opt-out via `dynamicrp.buildkit.enabled`) listening on `tcp://0.0.0.0:1234`. Add `buildctl-init` init container that copies the `buildctl` binary into an `emptyDir` mounted onto the dynamic-rp container's `PATH`. Add `dynamicrp.buildkit.psaMode` value, pod-level `fsGroup: 65532` in `restricted` mode, NOTES.txt preflight. No socket emptyDir, no BuildKit-state emptyDir, no Docker `config.json` Secret mount. |
| dynamic-rp recipe runner | `radius-project/radius` | Set `BUILDKIT_HOST=tcp://127.0.0.1:1234` and extend `PATH` with the `buildctl-init` mount in the recipe-execution environment. No Go code changes beyond environment plumbing. |
| Contributor documentation | `radius-project/radius` (`docs/contributing/`) | Add `buildkit-recipes.md` covering the buildkit subsystem and the `local-exec`-via-`buildctl` recipe pattern, so the next person adding a build-style recipe doesn't have to reverse-engineer it. |
| dynamic-rp context-upload endpoint | `radius-project/radius` | New endpoint accepting tarball uploads from the rad CLI; staged in an emptyDir for the recipe to consume. (Local development workflow, Option 2a.) |
| `rad` CLI local-context detection | `radius-project/radius` | When `build.context` is a local path, tar with `.dockerignore` honored and POST to dynamic-rp before recipe execution. |

### Error handling

| Failure | User experience |
|---|---|
| `restricted` mode on cluster without user namespaces | Helm preflight surfaces `Kubernetes ≥ 1.30 with UserNamespacesSupport required; reinstall without --set dynamicrp.buildkit.psaMode=restricted` and the install fails fast. |
| Multi-arch build with non-cross-compile-friendly Dockerfile | Build fails with `exec format error`. The recipe wraps it with a clear message: "Multi-arch build requested but the Dockerfile isn't cross-compile-friendly. Either rewrite using `FROM --platform=$BUILDPLATFORM` and `TARGETARCH`, or request a single platform." Nothing is pushed. |
| Registry credentials missing or incorrect | Push fails with the registry's auth error, surfaced through the recipe execution log. |
| BuildKit sidecar crash | Pod readiness fails; Kubernetes restarts the sidecar; the next recipe execution retries. |
| Local context upload exceeds size cap | rad CLI fails before the deploy with a message naming the offending paths and pointing at `.dockerignore` / git context as alternatives. |
| Source context unreachable (bad git URL) | BuildKit's frontend surfaces the git error; the recipe forwards it. |

## Test plan

* **Unit tests**: existing recipe-execution unit tests cover the
  Terraform run path; no new Go unit tests required.
* **Functional tests**:
  1. Single-arch build + push against a test registry.
  2. Multi-arch (cross-compile) build + push, verify manifest list.
  3. Non-cross-compile Dockerfile under multi-arch — expect the
     wrapped error with remediation message.

## Security

* No new privileged workloads; no host volume mounts; no added
  capabilities. The buildkitd sidecar runs as UID 1000.
* Default `baseline` mode is compatible with PSA `baseline` (most
  clusters' default) and works on every supported Kubernetes
  version. Operators who enforce PSA `restricted` cluster-wide
  opt in with `--set dynamicrp.buildkit.psaMode=restricted` on
  Kubernetes 1.30+ (stable in 1.33+).
* Registry credentials live in a `Radius.Security/secrets` of
  `kind: generic` (with `username` and `password` data keys),
  deployed by the PE into a dedicated `platform` application. Its
  recipe writes a Kubernetes Secret in the env's namespace; the
  containerImages recipe reads it via the `kubernetes_secret` data
  source named by the `registrySecretName` /
  `registrySecretNamespace` recipe parameters, composes a
  `config.json` for `buildctl`, and after a successful push creates
  a per-resource `kubernetes.io/dockerconfigjson` Secret in the
  developer's application namespace that `Radius.Compute/containers`
  references via `imagePullSecrets`. Credentials are never
  developer-Bicep parameters, chart-level Secret mounts, or
  out-of-band `kubectl create secret` invocations.
* The BuildKit endpoint is `tcp://127.0.0.1:1234` on the Pod's
  loopback interface and is not reachable from outside the Pod.
  No Service, NetworkPolicy egress, or Ingress is required.
* Build outputs are streamed directly to the user's registry; the
  build cache lives only in the sidecar's `emptyDir` and is lost
  when the Pod restarts. Cross-tenant cache poisoning is therefore
  not a concern at this scope.

## Compatibility

* No breaking changes to existing resource types or recipes.
* New Helm value `dynamicrp.buildkit.psaMode` defaults to
  `baseline`, which works on every supported Kubernetes version
  with no extra flags. Operators who want the stricter sidecar
  profile opt in with `--set dynamicrp.buildkit.psaMode=restricted`
  on Kubernetes 1.30+; the Helm preflight surfaces the required
  action if it's selected on an incompatible cluster.
* The image footprint of dynamic-rp grows by the size of the
  rootless BuildKit image (~80 MiB compressed at the time of
  writing).

## Monitoring and logging

* The buildkitd sidecar's stderr is captured by Kubernetes log
  collection like any other container.
* Recipe execution logs already capture Terraform's output,
  including the streamed stdout/stderr from the `buildctl`
  `local-exec` invocation (BuildKit's build progress).

## Development plan

| Workstream | Repo | Notes |
|---|---|---|
| Resource type schema | resource-types-contrib | New `containerImages.yaml`: `build` required, `tag` optional, `environment` and `application` optional, no per-resource `registry` override, no `image` field. Outputs: `image`, `imagePullSecretName`. Extend `Compute/containers/containers.yaml` with an `imagePullSecrets: array<string>` property. |
| Terraform recipe | resource-types-contrib | `main.tf` composes `<registry>/<resource>:<tag>`, content-hash tag default for local contexts, validates every interpolated input via `terraform_data "validate_inputs"` preconditions, requires explicit `properties.tag` for git contexts via `terraform_data "validate_git_tag"`, reads the registry Kubernetes Secret via the `kubernetes_secret` data source named by the `registrySecretName` / `registrySecretNamespace` recipe parameters, `base64decode`s `username` / `password`, renders `config.json` via `local_sensitive_file`, then shells out to `buildctl` via `local-exec` against `BUILDKIT_HOST`. After push, materializes a per-resource `kubernetes.io/dockerconfigjson` Secret in the application namespace and surfaces it as `imagePullSecretName`. The `Compute/containers` kubernetes recipe threads `imagePullSecrets` into the Deployment's `template.spec.image_pull_secrets`. |
| Sample recipe pack | resource-types-contrib (samples) | Example `Radius.Core/recipePacks` showing how to register the recipe with `registry`, `registrySecretName`, and `registrySecretNamespace` parameters. Companion PE Bicep deploys a `Radius.Core/applications` named `platform` and a `Radius.Security/secrets` of `kind: generic` (data: `username`, `password`) into it, and a sample developer Bicep declares `Radius.Compute/containerImages` with no credential plumbing and wires `imagePullSecrets: [demoImage.properties.imagePullSecretName]` into `Radius.Compute/containers`. |
| Helm chart sidecar | radius | Add buildkitd container listening on `tcp://0.0.0.0:1234`, `buildctl-init` init container copying `buildctl` into an `emptyDir` on the dynamic-rp container's `PATH`, `dynamicrp.buildkit.enabled` (default `true`) and `dynamicrp.buildkit.psaMode` values with `restricted` and `baseline` templates, pod-level `fsGroup: 65532` in `restricted` mode, NOTES.txt preflight. No socket/state emptyDir, no Docker `config.json` Secret mount. |
| Recipe-runner env plumbing | radius | Set `BUILDKIT_HOST` and extend `PATH` for the recipe execution. |
| Contributor documentation | radius | `docs/contributing/contributing-code/contributing-code-writing/buildkit-recipes.md`: explains the sidecar, the `buildctl-init` init container, the `local-exec`-via-`buildctl` recipe pattern, and the shell-injection-safety contract recipes are expected to follow. |
| Local-context upload (CLI ↔ dynamic-rp) | radius | rad CLI tarballs local `build.context`, POSTs to dynamic-rp; dynamic-rp stages for the recipe. (Local development workflow Option 2a.) |
| End-to-end test for `buildctl` ↔ rootless BuildKit | radius | **Resolved**: validated end-to-end (rootless BuildKit + buildctl + multi-arch + push to GHCR + digest into `Radius.Compute/containers`) in the demo repo before merging the chart change. |
| Functional test matrix | radius | Cross-platform CI: managed K8s (default `baseline` mode), opt-in `restricted` mode on K8s 1.33+, k3d. |

## Open questions

1. **Multi-arch failure semantics.** The proposed behavior is to
   fail loudly when a multi-arch build hits a Dockerfile that
   isn't cross-compile-friendly, with a clear remediation message.
   The alternatives — silent single-arch fallback, or silent
   native-only build — are arguably more "convenient" in the
   moment but produce runtime crashes on foreign-arch nodes. Is
   fail-loudly the right call, or should we offer a "best-effort"
   opt-in?
2. **Tag strategy default.** Content-addressable tags
   (`sha256-<hash>`) are correct for reconciliation but uglier in
   logs and registry UIs than `latest` or `<git-sha>`. Is the
   default acceptable, or should we offer a friendlier default
   (e.g. `<short-sha>` when context is git, content hash
   otherwise)?
3. **Local-context upload mechanism.** Option 2a (CLI → dynamic-rp
   HTTP). Not sure if this is actually possible or recommended.
4. **Idle sidecar cost.** ~50 MiB resident memory is paid by every
   dynamic-rp Pod, including those that never build images.
   Resolved: the sidecar is **default-on** with an
   opt-out Helm value (`dynamicrp.buildkit.enabled=false`) for
   operators who want the smaller footprint.
5. **Non-cross-compile Dockerfile support.** When does that become
   a priority? See appendix for the likely shape.
6. **Promotion schedule for `restricted` mode.** Worth setting an
   expectation for when (or whether) `restricted` should become
   the default — e.g. once Kubernetes 1.33+ is the minimum
   supported version across major managed K8s.
7. **Recipe vs. built-in resource provider.** **Resolved: ship as
   a Terraform recipe.** See
   [Implementation choice: recipe vs. built-in provider](#implementation-choice-recipe-vs-built-in-provider).
   The recipe preserves customization (signing, provenance,
   alternative tag schemes) and consistency with every other
   resource type. The special-casing that breaks the "no special
   cases" property is contained in dynamic-rp's chart and env
   (sidecar, `buildctl-init`, `BUILDKIT_HOST`), not in the recipe
   contract.

## Alternatives considered

The following were considered and rejected. Brief rationale only.

| # | Alternative | Why rejected |
|---|---|---|
| A | Mount the host Docker socket into dynamic-rp | Doesn't work on managed Kubernetes (no Docker socket on nodes); root-on-node mount; unacceptable as a long-term answer for a production resource type. |
| B | Sidecar running Docker-in-Docker | Requires `privileged: true` — strictly worse than the proposal. |
| C | Per-build Kubernetes Job that runs BuildKit | Source-context delivery to an in-cluster Job is the hard part; doesn't make multi-arch easier; adds RBAC and cleanup concerns. |
| D | `rad` CLI builds locally before deploying | Breaks the resource-type abstraction; moves build out of the recipe contract; differs between `rad deploy` and reconciliation. |
| E | Bundle QEMU and self-register `binfmt_misc` from dynamic-rp | Requires `CAP_SYS_ADMIN` permanently on a long-running service — the privilege escalation we are trying to eliminate. |
| F | Helm-managed one-shot privileged Job that registers `binfmt_misc` | Privileged workload in the chart; cluster admission policies often block it; Radius shouldn't own host kernel state. |
| G | QEMU emulation instead of cross-compile | Slow (5–20× native), unreliable for many Dockerfiles, and still requires either host binfmt registration or a privileged init container. |
| H | Long-lived `buildkitd` Service in its own namespace, recipes connect via remote driver + mTLS | Likely the right destination at scale (multi-tenant build service), but adds a Helm sub-chart, mTLS bootstrap, and multi-tenancy concerns. Premature for a first cut; revisit when usage justifies it. |
| I | Non-Docker builders (Buildah / Podman / Kaniko) | None remove the multi-arch constraint; weaker Terraform provider story; cross-compile-first design fits BuildKit best. Kaniko remains a fallback if a `restricted`-compatible build path is ever needed on Kubernetes versions that pre-date user namespaces. |
| J | Native multi-arch fan-out via the `buildx` Kubernetes driver | Assumes multi-arch node pools we are explicitly not assuming for v1; adds RBAC and a build namespace; would land an opt-in flag with no effect on the clusters targeted now. Kept as the natural extension; see appendix. |
| K | Env-level `recipeConfig.terraform.authentication.registries` analogous to `git.pat` | Requires a typespec change to `Radius.Core/environments`. The current design avoids that by deploying a generic `Radius.Security/secrets` from a PE-owned `platform` application and threading its name/namespace through `Radius.Core/recipePacks` parameters; the recipe resolves the Kubernetes Secret via a `kubernetes_secret` data source. Reconsider if first-class env-level registry credentials become a broader Radius concept. |

## Appendix: multi-architecture node pools

**Out of scope for this iteration.** This appendix records the
intended future story for two cases the v1 design intentionally
defers:

1. Dockerfiles that cannot be cross-compiled (target-arch `RUN`
   steps, language stacks without good cross-compile support,
   third-party Dockerfiles the user can't modify).
2. Clusters that already have multi-architecture node pools
   (e.g. EKS with Graviton, mixed AKS pools) and prefer native
   builds over cross-compile for cache locality or compile-time
   reasons.

The natural extension is **native fan-out** via BuildKit's `buildx`
Kubernetes driver. The recipe would gain an opt-in field — working
name `build.fanOut: true` — that, when set with multiple
`platforms`, dispatches one BuildKit Pod per requested architecture
to nodes of that architecture (via `nodeSelector` on
`kubernetes.io/arch`) and fans the per-arch outputs back into a
single manifest list. Each per-arch builder is still rootless; no
QEMU/binfmt is involved.

Why deferred:

* It assumes cluster shape (multi-arch node pools) v1 explicitly
  does not assume.
* It requires extra RBAC (a build namespace, a Role allowing
  buildx to create/delete builder Pods) that adds install
  complexity.
* It would land an opt-in flag that has no effect on the clusters
  targeted now, which is worse than not having the flag.

If a user requests multiple architectures and the Dockerfile can't
be cross-compiled, v1 surfaces a clear error. The fix is either to
make the Dockerfile cross-compile-friendly or to wait for fan-out
support. There is no QEMU fallback.
