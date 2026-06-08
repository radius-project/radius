# Pod Security Admission notes for the dynamic-rp BuildKit sidecar

The `Radius.Compute/containerImages` resource type relies on a rootless
BuildKit sidecar in the `dynamic-rp` Pod. This document captures what
we empirically validated about how that sidecar interacts with
Kubernetes Pod Security Admission, so operators can configure their
clusters accordingly.

## TL;DR

| PSA mode (radius-system namespace) | BuildKit sidecar status |
|------------------------------------|-------------------------|
| `privileged` (default for unlabeled namespaces) | ✅ Supported, validated end-to-end |
| `baseline` | ❌ Not supported |
| `restricted` | ❌ Not supported |

Operators who need their cluster to enforce PSA `baseline` or
`restricted` cluster-wide should:

1. Keep `Radius.Compute/containerImages` disabled
   (`--set dynamicrp.buildkit.enabled=false`), and label radius-system
   accordingly. The chart will install cleanly under `baseline` /
   `restricted` without the sidecar.
2. If the resource type IS needed, label only the `radius-system`
   namespace as `privileged`:
   `kubectl label ns radius-system pod-security.kubernetes.io/enforce=privileged --overwrite`.
   The resource type only requires elevated privilege within that
   namespace; user workload namespaces (where `Radius.Compute/containers`
   actually runs the built images) can stay `baseline` / `restricted`.

## Why baseline/restricted don't work

BuildKit's official Kubernetes guidance offers two configurations:

- `pod.rootless.yaml`: `moby/buildkit:*-rootless` image.
  Requires `seccompProfile: Unconfined` AND `appArmorProfile: Unconfined`.
  Both are forbidden by PSA `baseline`.
- `pod.userns.yaml`: `moby/buildkit:*` (non-rootless) image with
  `hostUsers: false` and `privileged: true`. `privileged: true` is
  forbidden by PSA `baseline`.

We tried a third configuration: non-rootless image + `hostUsers: false`
without `privileged: true`. PSA `baseline` accepts the pod, the
buildkitd process starts and serves, but actual image builds fail
inside BuildKit's snapshotter / runc machinery with cryptic
"operation not permitted" errors. This is an upstream gap in
BuildKit's K8s user-namespace support: the
`--oci-worker-no-process-sandbox` flag that K8s deployments use to
work around the lack of `--security-opt systempaths=unconfined` is
gated to the rootless image only.

(Validation runs: AKS PSA baseline workflow #27160081225, EKS PSA
baseline workflow #27164309017 / #27164822141, k3d PSA baseline
workflow #27154219074. Full debug logs in the demo repo's actions
history.)

## What about Localhost seccomp profiles?

PSA `baseline` accepts `seccompProfile.type: Localhost` (only forbids
`Unconfined`), so in principle a custom seccomp profile that allows
the syscalls rootlesskit needs (mount, pivot_root, unshare with
`CLONE_NEWUSER`, etc.) but is otherwise restrictive could work.
Implementing this requires:

- A node-side seccomp profile JSON installed at
  `/var/lib/kubelet/seccomp/` on every node.
- Either a separate DaemonSet shipping it, or out-of-band node
  provisioning.

We have not built or validated this approach. Operators with an
existing seccomp-profile distribution mechanism may pursue it as a
cluster-specific extension; we'd be interested in PRs.

## Future work to revisit baseline support

- BuildKit upstream needs to add a non-rootless K8s configuration
  that doesn't require `privileged: true`. Tracking issues:
  - https://github.com/moby/buildkit/blob/master/examples/kubernetes/pod.userns.yaml
- Kubernetes user-namespace defaults (`hostUsers: false`) become
  cluster-default in K8s 1.36+ (GA April 2026), which may shift
  upstream BuildKit's stance.
