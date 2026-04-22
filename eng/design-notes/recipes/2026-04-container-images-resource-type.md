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

This document is the definitive design for the resource type. It
covers the user-facing schema, the Terraform recipe that implements
it, and how dynamic-rp executes the build inside a Kubernetes
cluster without privileged workloads or host-side prerequisites.

## Terms and definitions

| Term | Definition |
|---|---|
| **BuildKit** | The container build engine that modern Docker uses internally. Has a maintained "rootless" container image suitable for running inside Kubernetes Pods. |
| **buildkitd** | The BuildKit daemon. Listens on a unix or TCP socket and accepts build requests from clients (the Docker CLI, the Terraform Docker provider, `buildctl`, etc.). |
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
* Implement the resource type with a Terraform recipe that uses the
  existing `kreuzwerker/docker` provider, so the recipe contract is
  unchanged.
* Build images on **any Kubernetes cluster** Radius supports —
  managed (EKS / AKS / GKE), self-hosted, and local (k3d / kind).
* Avoid host privilege: no host volume mounts, no `privileged: true`,
  no added Linux capabilities, no host networking, no host kernel
  preparation.
* Support multi-architecture builds in the common case
  (cross-compile-friendly Dockerfiles).
* Default to a Pod security posture compatible with PSA `restricted`
  on modern clusters.

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

### User scenarios

#### User story 1 — Developer iterates on an image locally

A developer has a service in `./frontend` of their working tree.
They add a `containerImages` resource to their Bicep file, set
`build.context: './frontend'`, and reference the resulting image
from a `containers` resource. `rad deploy` tarballs the local
directory, uploads it to dynamic-rp, builds via the in-cluster
BuildKit, pushes to the recipe-configured registry, and rolls the
container. Inner-loop iteration uses no out-of-band `docker build`
or `docker push`.

#### User story 2 — Developer builds from a git URL

A developer has already pushed their code to a git repository and
wants to build directly from there instead of uploading their
working tree. The same Bicep, but
`build.context: 'git::https://github.com/alice/myapp.git//frontend'`.
BuildKit clones the repo inside the cluster on each deployment; no
local context upload is needed.

#### User story 3 — Multi-architecture image on a single-arch cluster

A developer working on an amd64-only AKS cluster needs both
`linux/amd64` and `linux/arm64` images for downstream environments.
They list both in `build.platforms`. Their Dockerfile uses
`FROM --platform=$BUILDPLATFORM` and `TARGETARCH`, so both
architectures build cross-compile on the amd64 builder and a
manifest list is pushed.

#### User story 4 — Operator installs Radius on a regulated cluster

An operator runs Radius on a cluster that enforces PSA `restricted`
cluster-wide. They install Radius with the default Helm values; the
buildkitd sidecar runs in `restricted`-compatible mode (Kubernetes
user namespaces) without policy exceptions.

## User Experience

### Sample input

```bicep
extension radius
extension containerImages

param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'myapp'
  properties: { environment: environment }
}

resource frontendImage 'Radius.Compute/containerImages@2025-08-01-preview' = {
  name: 'todolist-app'
  properties: {
    environment: environment
    application: app.id
    build: {
      context: 'git::https://github.com/alice/myapp.git//frontend'
      platforms: ['linux/amd64', 'linux/arm64']
    }
  }
}

resource frontend 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'frontend'
  properties: {
    environment: environment
    application: app.id
    containers: {
      app: {
        image: frontendImage.properties.image
        ports: { web: { containerPort: 3000 } }
      }
    }
  }
}
```

The developer never writes the registry, the repository path, or
the tag. The final image reference is composed by the recipe:

```
<registry>/<resource-name>:<tag>
ghcr.io/alice/todolist-app:sha256-d4f2…
└──────┬────┘ └─────┬────┘ └─────┬─────┘
   from registry    from the     content-addressable
   recipe param   resource name  tag (default)
```

Tags default to a content-addressable digest (see [Tag strategy](#tag-strategy)).
Developers can override per-resource by setting `properties.tag`.

### Recipe registration

The platform engineer registers the recipe once per environment,
supplying the registry **base** (registry host plus an optional
namespace/org segment) and credentials as recipe parameters:

```sh
rad recipe register default \
  --resource-type Radius.Compute/containerImages \
  --template-kind terraform \
  --template-path git::https://github.com/radius-project/resource-types-contrib.git//Compute/containerImages/recipes/kubernetes/terraform \
  --parameters registry=ghcr.io/alice \
  --parameters registry_username=alice \
  --parameters registry_token=<PAT>
```

The `registry` parameter is the prefix; the resource name supplies
the final path segment. Different environments can register the
same recipe against different registries (dev → `ghcr.io/alice`,
staging → `myorg.azurecr.io/staging`, prod → `myorg.azurecr.io/prod`)
without any change to the developer's Bicep.

Developers do not see or manage credentials.

### Sample output

`rad deploy` reports build progress through the recipe execution log
and produces an image reference at `properties.image`. Downstream
resources that consume that output redeploy with the new digest on
the next reconciliation.

## Design

### High-level design

A `Radius.Compute/containerImages` resource is reconciled by
dynamic-rp like any other recipe-backed resource type. The recipe is
written in Terraform and uses the `kreuzwerker/docker` provider.
What makes this resource type different from existing recipes is
that the Docker provider needs a build endpoint to talk to. To
provide one without depending on the host (which is unreachable on
managed Kubernetes), Radius runs **rootless BuildKit as a sidecar
container in the dynamic-rp Pod** and exposes its unix socket to
recipe execution via a shared volume.

1. **Resource type schema** (this document, and `containerImages.yaml`):
   defines the user-facing API.
2. **Terraform recipe** (this document, and `recipes/kubernetes/terraform/`):
   takes the resource's properties, calls the Docker provider
   against the local BuildKit endpoint, builds, and pushes.
3. **dynamic-rp Helm chart** (this document, and `deploy/Chart`):
   adds the buildkitd sidecar and the shared socket volume so the
   recipe has something to talk to.

### Architecture diagram

```
┌─────────────────────────── dynamic-rp Pod ───────────────────────────┐
│                                                                      │
│  ┌────────────────────────┐         ┌──────────────────────────┐    │
│  │  dynamic-rp container  │         │  buildkitd container     │    │
│  │                        │         │  (moby/buildkit:rootless)│    │
│  │  ┌──────────────────┐  │         │                          │    │
│  │  │ recipe execution │──┼────────▶│  unix:///run/buildkit/   │    │
│  │  │ (Terraform +     │  │  gRPC   │  buildkit.sock           │    │
│  │  │  docker provider)│  │         │                          │    │
│  │  └──────────────────┘  │         └────────────┬─────────────┘    │
│  └────────────────────────┘                      │                  │
│                                                  │                  │
│         shared emptyDir volumes:                 │                  │
│           /run/buildkit  (socket)                │                  │
│           /home/user/.local/share/buildkit       │                  │
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
| `environment` | string | yes | The Radius Environment ID. |
| `application` | string | yes | The Radius Application ID. |
| `tag` | string | no | Tag for the produced image. Defaults to a content-addressable digest computed from the build inputs (see [Tag strategy](#tag-strategy)). |
| `build.context` | string | yes | Source location. Either a git URL (`git::https://…`) or — for local development workflows — a path that the rad CLI uploads as a tarball. See [Local development workflow](#local-development-workflow). |
| `build.dockerfile` | string | no | Path to the Dockerfile relative to the context. Defaults to `Dockerfile`. |
| `build.platforms` | string[] | no | Target platforms (e.g. `["linux/amd64", "linux/arm64"]`). When omitted, builds for the BuildKit sidecar's native architecture. |
| `registry` | string | no | Per-resource override of the recipe's `registry` parameter. Most developers leave this unset. |

The resource **name** (e.g. `todolist-app`) is what the developer
writes in `resource <name> 'Radius.Compute/containerImages@…'`, and
becomes the final path segment of the image reference.

##### Outputs

| Output | Description |
|---|---|
| `properties.image` | The full resolved image reference, e.g. `ghcr.io/alice/todolist-app:sha256-d4f2…`. Downstream `Radius.Compute/containers` resources reference this so they pick up new digests automatically. |

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

* **Provider**: `kreuzwerker/docker` ≥ 3.0. The provider speaks
  BuildKit gRPC natively; it does not need a Docker CLI on the
  recipe runner.
* **Endpoint**: configured via the `DOCKER_HOST` environment
  variable, which dynamic-rp sets to
  `unix:///run/buildkit/buildkit.sock` for recipe execution. The
  recipe itself does not encode the endpoint.
* **Authentication**: reads `~/.docker/config.json` from the
  recipe-runner filesystem. The dynamic-rp Pod mounts a Secret
  containing the credentials configured by the platform engineer at
  recipe-registration time.
* **Recipe parameters** (set by the platform engineer at
  registration time): `registry` (e.g. `ghcr.io/alice`),
  `registry_username`, `registry_token`.

Sketch of the resources the recipe declares:

```hcl
provider "docker" {
  registry_auth {
    address     = local.registry_host
    config_file = pathexpand("~/.docker/config.json")
  }
}

locals {
  resource_name = var.context.resource.name
  registry      = coalesce(
    try(var.context.resource.properties.registry, null),
    var.registry,
  )
  registry_host = regex("^[^/]+", local.registry)

  context_sha   = sha256(...)             # over context + dockerfile + platforms
  resolved_tag  = coalesce(
    try(var.context.resource.properties.tag, null),
    "sha256-${substr(local.context_sha, 0, 16)}",
  )
  image_ref     = "${local.registry}/${local.resource_name}:${local.resolved_tag}"
}

resource "docker_image" "build" {
  name = local.image_ref
  build {
    context    = local.build_context
    dockerfile = local.dockerfile
    platform   = length(local.platforms) > 0 ? join(",", local.platforms) : null
  }
  triggers = { src_sha = local.context_sha }
}

resource "docker_registry_image" "push" {
  name          = docker_image.build.name
  keep_remotely = true
}

output "properties" {
  value = { image = local.image_ref }
}
```

Multi-arch is handled by passing multiple platforms in
`build.platform`; BuildKit produces a manifest list and pushes it.
No buildx-builder resource is required because the recipe always
talks to a single rootless BuildKit endpoint and uses cross-compile
for foreign architectures.

#### dynamic-rp Helm chart changes

The chart change has three parts: adding the sidecar, sharing the
socket, and choosing a Pod security profile.

**1. Sidecar container.** Add a second container to the dynamic-rp
Deployment, using the upstream
`moby/buildkit:<pinned-version>-rootless` image. The sidecar's
liveness/readiness probes use `buildctl debug workers`, matching
upstream's recommended manifest.

**2. Shared volumes.** Two `emptyDir` volumes mounted into both
containers:
* `/run/buildkit` — holds the unix socket the recipe talks to.
* `/home/user/.local/share/buildkit` — BuildKit's working state
  directory (mounted to satisfy upstream's documented requirement
  on Container-Optimized OS).

The dynamic-rp container also mounts a Secret-backed volume at
`/home/dynamicrp/.docker` containing `config.json`.

**3. Pod security profile.** Selected by the Helm value
`dynamicrp.buildkit.psaMode`, with two settings sharing the same
image and socket:

| Mode | Pod / sidecar security controls | When to use |
|---|---|---|
| **`restricted`** (default) | `pod.spec.hostUsers: false`. Sidecar has no `Unconfined` profiles, no `--oci-worker-no-process-sandbox`. Compatible with PSA `restricted`. | Default. Requires Kubernetes user namespaces (stable in 1.33+, beta on-by-default in 1.30+). |
| **`baseline`** | Sidecar sets `seccompProfile: Unconfined`, `appArmorProfile: Unconfined`, args `--oci-worker-no-process-sandbox`. Compatible with PSA `baseline`. | Clusters older than 1.30 or where user namespaces are disabled. |

Neither mode uses `privileged: true`, mounts host paths, or
requires added Linux capabilities. The default targets the
dominant security posture of clusters provisioned today: as of
April 2026, EKS, AKS, and GKE all support Kubernetes 1.33+, and
that version will be the comfortable minimum within the lifetime
of this resource type.

The chart includes a Helm `NOTES.txt` preflight that surfaces a
clear message ("Kubernetes ≥ 1.30 with UserNamespacesSupport
required; rerun with `--set dynamicrp.buildkit.psaMode=baseline`")
if `restricted` is selected on an incompatible cluster.

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
* The `kreuzwerker/docker` Terraform provider speaks BuildKit's
  protocol natively, so the recipe contract does not change.
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
* Default install is compatible with PSA `restricted` on Kubernetes
  1.33+, matching the security posture of most newly-provisioned
  managed clusters.
* The recipe contract is unchanged: same Terraform provider as any
  other Docker-targeted recipe, no new build-tool-specific
  abstractions.
* Single image for both PSA modes; differences are limited to the
  Pod's `securityContext` block.

### Disadvantages

* **Two PSA profiles to support.** `restricted` (default) requires
  Kubernetes user namespaces. Operators on older Kubernetes set
  `dynamicrp.buildkit.psaMode=baseline` at install time and need
  the install namespace to permit PSA `baseline`. The chart fails
  fast with a clear message if `restricted` is selected on an
  incompatible cluster.
* **Idle resource cost.** The buildkitd sidecar adds ~50 MiB
  resident memory to every dynamic-rp Pod, whether
  `containerImages` is used or not.
* **Dockerfiles must be cross-compile-friendly for multi-arch.**
  Multi-architecture builds rely on `BUILDPLATFORM` /
  `TARGETPLATFORM`. Dockerfiles that execute target-arch binaries
  during the build won't work multi-arch under this design and will
  fail at build time with a clear error.
* **Credential bootstrap.** The recipe needs registry credentials
  delivered into the Pod via a Kubernetes Secret. Defining the
  secret-management UX is a separate workstream.
* **Local-context upload size.** The CLI tarball-upload path
  (Option 2 in [Local development workflow](#local-development-workflow))
  scales poorly for very large directories. `.dockerignore` and a
  reasonable size cap mitigate but don't eliminate this. Multi-GiB
  contexts probably need git.

### Implementation details

| Component | Repo | Change |
|---|---|---|
| Resource type schema | `radius-project/resource-types-contrib` | Add `Compute/containerImages/containerImages.yaml`. |
| Terraform recipe | `radius-project/resource-types-contrib` | Add `Compute/containerImages/recipes/kubernetes/terraform/{main.tf,var.tf}`. Recipe targets the BuildKit unix socket via `DOCKER_HOST`; composes image ref from `registry`, resource name, and content-hash tag. |
| dynamic-rp Helm chart | `radius-project/radius` (`deploy/Chart`) | Add `buildkitd` sidecar, `emptyDir` volumes for socket and BuildKit state, Secret-backed credentials volume, `dynamicrp.buildkit.psaMode` value, NOTES.txt preflight. |
| dynamic-rp recipe runner | `radius-project/radius` | Set `DOCKER_HOST=unix:///run/buildkit/buildkit.sock` in the recipe-execution environment. No Go code changes beyond environment plumbing. |
| dynamic-rp context-upload endpoint | `radius-project/radius` | New endpoint accepting tarball uploads from the rad CLI; staged in an emptyDir for the recipe to consume. (Local development workflow, Option 2a.) |
| `rad` CLI local-context detection | `radius-project/radius` | When `build.context` is a local path, tar with `.dockerignore` honored and POST to dynamic-rp before recipe execution. |

### Error handling

| Failure | User experience |
|---|---|
| `restricted` mode on cluster without user namespaces | Helm preflight surfaces `Kubernetes ≥ 1.30 with UserNamespacesSupport required; rerun with --set dynamicrp.buildkit.psaMode=baseline` and the install fails fast. |
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
* Default `restricted` mode is compatible with PSA `restricted` and
  does not require any cluster-policy exception on K8s 1.33+.
* `baseline` mode requires the dynamic-rp namespace to permit PSA
  `baseline` (most clusters' default). Operators that enforce
  `restricted` cluster-wide must either be on K8s 1.30+ (use
  default mode) or namespace-exempt the dynamic-rp install.
* Registry credentials live in a Kubernetes Secret in the
  dynamic-rp namespace and are mounted into the sidecar. They are
  not visible to recipes from other resource types.
* The BuildKit socket is on an `emptyDir` shared between the two
  containers in the same Pod. It is not exposed outside the Pod.
* Build outputs are streamed directly to the user's registry; the
  build cache lives only in the sidecar's `emptyDir` and is lost
  when the Pod restarts. Cross-tenant cache poisoning is therefore
  not a concern at this scope.

## Compatibility

* No breaking changes to existing resource types or recipes.
* New Helm value `dynamicrp.buildkit.psaMode` defaults to
  `restricted`. Operators who upgrade Radius onto an older cluster
  must explicitly set `=baseline`. The Helm preflight surfaces the
  required action.
* The image footprint of dynamic-rp grows by the size of the
  rootless BuildKit image (~80 MiB compressed at the time of
  writing).

## Monitoring and logging

* The buildkitd sidecar's stderr is captured by Kubernetes log
  collection like any other container.
* Recipe execution logs already capture Terraform's output,
  including BuildKit's build progress (Terraform's Docker provider
  forwards it).

## Development plan

| Workstream | Repo | Notes |
|---|---|---|
| Resource type schema | resource-types-contrib | New `containerImages.yaml` with `tag` optional, `registry` optional override, no `image` field. |
| Terraform recipe | resource-types-contrib | `main.tf` composes `<registry>/<resource>:<tag>`, content-hash tag default, talks to BuildKit unix socket via `DOCKER_HOST`. |
| Helm chart sidecar | radius | Add buildkitd container, emptyDir volumes, Secret mount, `psaMode` value with `restricted` and `baseline` templates, NOTES.txt preflight. |
| Recipe-runner env plumbing | radius | Set `DOCKER_HOST` for the recipe execution. |
| Local-context upload (CLI ↔ dynamic-rp) | radius | rad CLI tarballs local `build.context`, POSTs to dynamic-rp; dynamic-rp stages for the recipe. (Local development workflow Option 2a.) |
| End-to-end test for Docker provider ↔ rootless BuildKit | radius | Highest-risk unknown. Validate before merging the chart change. |
| Functional test matrix | radius | Cross-platform CI: managed K8s (default mode), older K8s (baseline mode), k3d. |

## Open questions

1. **Default `psaMode`.** Defaulting to `restricted` matches the
   security posture of K8s 1.33+ clusters but means operators on
   older K8s see an install-time failure unless they pass
   `--set dynamicrp.buildkit.psaMode=baseline`. Is the explicit
   opt-in for older clusters acceptable, or should the chart
   auto-detect?
2. **Multi-arch failure semantics.** The proposed behavior is to
   fail loudly when a multi-arch build hits a Dockerfile that
   isn't cross-compile-friendly, with a clear remediation message.
   The alternatives — silent single-arch fallback, or silent
   native-only build — are arguably more "convenient" in the
   moment but produce runtime crashes on foreign-arch nodes. Is
   fail-loudly the right call, or should we offer a "best-effort"
   opt-in?
3. **Tag strategy default.** Content-addressable tags
   (`sha256-<hash>`) are correct for reconciliation but uglier in
   logs and registry UIs than `latest` or `<git-sha>`. Is the
   default acceptable, or should we offer a friendlier default
   (e.g. `<short-sha>` when context is git, content hash
   otherwise)?
4. **Local-context upload mechanism.** Option 2a (CLI → dynamic-rp
   HTTP). Not sure if this is actually possible or recommended.
5. **Idle sidecar cost.** ~50 MiB resident memory is paid by every
   dynamic-rp Pod, including those that never build images.
   Acceptable, or should the sidecar be conditional on operator
   opt-in?
6. **Docker provider ↔ rootless BuildKit compatibility.** Verified
   by source inspection but not end-to-end. Highest-risk unknown.
7. **Non-cross-compile Dockerfile support.** When does that become
   a priority? See appendix for the likely shape.
8. **Deprecation schedule for `baseline` mode.** Worth setting an
   expectation now (e.g., when 1.30 reaches end-of-support across
   major managed K8s)?

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

## Design Review Notes

_To be filled in during design review._

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
