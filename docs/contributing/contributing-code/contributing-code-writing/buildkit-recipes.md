# BuildKit sidecar and `local-exec` recipes

This page explains the optional BuildKit sidecar that ships with the
`dynamic-rp` chart and the recipe pattern it enables. Read this before
modifying the buildkit values surface, or before authoring a new
recipe that needs to shell out to a CLI tool inside the dynamic-rp
Pod.

The motivating consumer is the `Radius.Compute/containerImages`
resource type ([design
doc](../../../../eng/design-notes/recipes/2026-04-container-images-resource-type.md)),
but the pattern generalizes.

## Architecture

The chart adds two things to the dynamic-rp Pod when
`dynamicrp.buildkit.enabled=true` (the default):

1. A **`buildkitd` sidecar container** running rootless
   [`moby/buildkit`](https://github.com/moby/buildkit) and listening
   on Pod loopback TCP.
2. A **`buildctl-init` init container** that copies the `buildctl` CLI
   into a shared `emptyDir`, which is then mounted into the dynamic-rp
   container's `PATH`.

A **registry-credentials volume** also mounts an operator-supplied
Secret at `~/.docker/config.json` inside the dynamic-rp container, so
any tool that reads Docker credential files (including `buildctl`)
authenticates transparently.

The dynamic-rp container itself is otherwise unmodified — it still
runs Terraform recipes, just now with `buildctl` on `PATH` and a
working buildkitd to dial.

```
┌─────────────────────────────────────────────────────────────┐
│ dynamic-rp Pod                                              │
│                                                             │
│  buildctl-init ──► copies /usr/bin/buildctl to shared vol   │
│                                                             │
│  ┌──────────────────────┐   loopback   ┌──────────────────┐ │
│  │ dynamic-rp container │ ───TCP─────► │ buildkitd        │ │
│  │ (Terraform recipes)  │              │ (rootless)       │ │
│  └──────────────────────┘              └──────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Values surface

```yaml
dynamicrp:
  buildkit:
    enabled: true              # set false to drop the sidecar entirely
    psaMode: restricted        # "restricted" or "baseline"
    image: moby/buildkit:vX.Y.Z-rootless
    credentialsSecret: ""      # name of a Secret in the dynamic-rp namespace
    resources:
      limits: { cpu: "2", memory: 4Gi }
      requests: { cpu: 100m, memory: 256Mi }
```

- **`psaMode`** must match the Pod Security Admission level enforced
  on the namespace. `restricted` requires Kubernetes ≥ 1.30 with the
  `UserNamespacesSupport` feature gate (uses `hostUsers: false` and
  drops all capabilities). `baseline` works on older clusters.
- **`credentialsSecret`** is a Secret with a `config.json` key holding
  a Docker config-format credentials document. Without it, builds can
  still run but pushes will fail with `unauthorized`. The chart's
  `NOTES.txt` warns when it is unset.
- **`resources`** defaults are sized for small images on a developer
  cluster. Tune up for production workloads — buildkitd is the
  hot-path consumer.

## The `local-exec` recipe pattern

Recipes can shell out to `buildctl` (or any CLI on the dynamic-rp
container's `PATH`) using Terraform's
[`terraform_data` + `local-exec`
provisioner](https://developer.hashicorp.com/terraform/language/resources/provisioners/local-exec).
The canonical reference implementation is
`Compute/containerImages/recipes/kubernetes/terraform/main.tf` in
[`resource-types-contrib`](https://github.com/radius-project/resource-types-contrib).

Things to get right when authoring such a recipe:

### Shell-injection safety

Every value interpolated into the `command` heredoc lands unquoted on
a `/bin/sh -c` command line. Validate every user-controlled input
against a tight regex in a `precondition` block on a separate
`terraform_data` resource, then make the build resource
`depends_on` it. See `validate_inputs` in `containerImages/main.tf`.

### Triggering on content, not metadata

`local-exec` provisioners only run when the resource is created or
replaced. Use `triggers_replace` on the `terraform_data` resource to
hash the inputs that should cause a rebuild — typically the image
reference plus a content hash of the build context. Don't rely on
file timestamps; they're meaningless inside a recipe Pod.

### Destroy semantics

Provisioners by default also run on destroy, which is rarely what you
want for build-and-push workflows. Be explicit in the README about
what `terraform destroy` does and does not clean up. For
`containerImages`, destroy intentionally does **not** delete the
pushed image (registry retention is a separate concern).

### Failure visibility

`local-exec` writes `stdout`/`stderr` to the Terraform log at INFO
level. The dynamic-rp wrapper surfaces this through the recipe
deployment's status. Prefer `set -eu` at the top of every command and
let buildctl's own diagnostics do the work; don't swallow output with
`2>/dev/null`.

## Modifying the chart subsystem

The contract between the chart and `pkg/recipes/terraform` is the
on-disk path of the pre-mounted Terraform binary. Constants in
`pkg/recipes/terraform/install.go` (`defaultGlobalTerraformDir`,
`defaultGlobalTerraformBinary`, `defaultGlobalTerraformMarkerFile`)
must stay in lockstep with the path the chart's init script writes
to. A drift-guard test in `deploy/Chart/tests/helpers_test.yaml`
asserts the chart still references those paths; update both sides
together if you ever rename the directory.

When extending the buildkit values surface (e.g. adding a new tunable
under `dynamicrp.buildkit.*`), also:

1. Add a default in `deploy/Chart/values.yaml` with an inline comment.
2. Wire it through `deploy/Chart/templates/dynamic-rp/deployment.yaml`.
3. Add a helm-unittest case under `deploy/Chart/tests/` that asserts
   the rendered manifest both with the default and with an override.
