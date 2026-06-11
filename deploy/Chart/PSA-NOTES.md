# Pod Security Admission notes for the dynamic-rp BuildKit sidecar

The `Radius.Compute/containerImages` resource type depends on a
rootless BuildKit sidecar in the `dynamic-rp` Pod. The sidecar is
opt-in (`dynamicrp.buildkit.enabled: false` by default in the
chart). This document describes the PSA implications for operators
deciding whether to enable it.

## TL;DR

| PSA mode on `radius-system` | Sidecar disabled (default) | Sidecar enabled |
|---|---|---|
| `privileged` | Works | Works |
| `baseline` | Works | Sidecar rejected at admission |
| `restricted` | Chart not supported (unrelated to this feature) | Chart not supported (unrelated to this feature) |

The default install is PSA-baseline-compatible because the sidecar is
not installed. Operators who want `Radius.Compute/containerImages`
need to enable the sidecar and either run radius-system under PSA
`privileged` or accept that admission will reject the dynamic-rp Pod.

## Enabling the sidecar

```bash
rad install kubernetes --set dynamicrp.buildkit.enabled=true
# or
helm install radius radius/radius --set dynamicrp.buildkit.enabled=true
```

If `radius-system` is unlabeled (the default on AKS, EKS, GKE, kind,
k3d), no further action is needed: unlabeled namespaces inherit PSA
`privileged`.

If your cluster enforces PSA `baseline` on `radius-system` (either by
namespace label or cluster-wide API server default), explicitly label
`radius-system` as `privileged`:

```bash
kubectl label ns radius-system pod-security.kubernetes.io/enforce=privileged --overwrite
```

Workload namespaces (where `Radius.Compute/containers` actually
deploys the built images) are unaffected and can remain at
`baseline` or `restricted`.

## Why baseline doesn't work for the sidecar

Building OCI images inside a Kubernetes Pod requires Linux
capabilities (mount, unshare, pivot_root, /proc manipulation) that
PSA baseline disallows. BuildKit's maintainer (AkihiroSuda, also the
author of rootlesskit) confirmed this is fundamental in
[moby/buildkit#4022](https://github.com/moby/buildkit/issues/4022):

> The default apparmor profile prohibits mounting, so it still
> cannot be enabled.

Per the
[PSA spec](https://kubernetes.io/docs/concepts/security/pod-security-standards/),
baseline forbids:

- `seccompProfile.type: Unconfined` (rootless BuildKit requires it
  for rootlesskit's `unshare`)
- `appArmorProfile.type: Unconfined` (rootless BuildKit requires it
  for `mount`)
- Capabilities outside the allow-list (the allow-list excludes
  `SYS_ADMIN`, required for in-pod image builds)
- `procMount: Unmasked`

Every in-cluster image builder hits at least one of these.

## Validation

Verified empirically with PSA enforcement on:

- EKS PSA baseline workflow: admission passes, daemon starts, image
  builds fail in the snapshotter machinery (workflow run
  [#27164309017](https://github.com/willdavsmith/radius-containerimagetype-demo/actions/runs/27164309017))
- k3d PSA baseline workflow: admission passes, daemon blocked by the
  inner-Docker AppArmor profile
- AKS PSA baseline workflow: blocked on OIDC service principal
  permissions in the test environment

Full debug logs in the
[demo repo Actions history](https://github.com/willdavsmith/radius-containerimagetype-demo/actions).

## Future paths

If the PSA `baseline` limitation becomes a blocker for in-cluster
builds, possible directions:

- Localhost seccomp profile via a node DaemonSet. PSA baseline
  accepts `seccompProfile.type: Localhost`, so a custom profile that
  allows BuildKit's syscall set could work. Requires shipping a JSON
  profile to every node's `/var/lib/kubelet/seccomp/` directory.
  Untested.
- Out-of-cluster builds. Move image building to an external CI/CD
  pipeline. Radius config references the pre-built image directly.
  Sidesteps the PSA problem entirely.
- K8s 1.36+ user namespaces (GA April 2026). May shift upstream
  BuildKit's stance on requiring Unconfined seccomp. Worth
  re-evaluating in 6-12 months once Linux distros and BuildKit catch
  up.

## References

- [moby/buildkit#4022](https://github.com/moby/buildkit/issues/4022).
  Confirms AppArmor is the fundamental blocker.
- [moby/buildkit#3217](https://github.com/moby/buildkit/issues/3217).
  Same constraint manifesting on GKE Autopilot.
- [Kubernetes Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/).
- [BuildKit rootless docs](https://github.com/moby/buildkit/blob/master/docs/rootless.md).
  Lists every required `--security-opt`.

