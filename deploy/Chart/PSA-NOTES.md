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
| `baseline` | ❌ Not supported (upstream BuildKit limitation) |
| `restricted` | ❌ Not supported (upstream BuildKit limitation) |

Operators who need their cluster to enforce PSA `baseline` or
`restricted` cluster-wide should:

1. Keep `Radius.Compute/containerImages` disabled
   (`--set dynamicrp.buildkit.enabled=false`). The chart will install
   cleanly under `baseline` / `restricted` without the sidecar.
2. If the resource type IS needed, label only the `radius-system`
   namespace as `privileged`:
   `kubectl label ns radius-system pod-security.kubernetes.io/enforce=privileged --overwrite`.
   The resource type only requires elevated privilege within that
   namespace; user workload namespaces (where `Radius.Compute/containers`
   actually runs the built images) can stay `baseline` / `restricted`.

## Why baseline/restricted don't work

This is **not a Radius-specific problem.** Building OCI images inside
a Kubernetes Pod requires Linux capabilities (mount, unshare,
pivot_root, /proc manipulation) that PSA baseline and restricted both
disallow. BuildKit's maintainer (AkihiroSuda, also the author of
rootlesskit) has confirmed this is fundamental:

> "The default apparmor profile prohibits mounting, so it still
> cannot be enabled."
> — moby/buildkit#4022 (the upstream issue asking exactly this question)

The same constraint applies to alternative builders:

- **Kaniko**: ARCHIVED June 2025; no longer maintained.
- **img** (`genuinetools/img`): unmaintained since 2024.
- **Buildah**: needs `seccompProfile: Unconfined` for rootless mounts;
  PSA baseline forbids Unconfined seccomp.
- **BuildKit rootless**: needs Unconfined seccomp + AppArmor for
  rootlesskit's newuidmap/unshare; both forbidden by baseline.
- **BuildKit + `hostUsers: false` + non-rootless image**: chart
  admission passes baseline AND the daemon starts, BUT actual image
  builds fail in the snapshotter machinery (BuildKit's per-build-step
  PID-namespace creation hits seccomp restrictions; the
  `--oci-worker-no-process-sandbox` workaround is gated to the
  rootless image only). Tracked as the same constraint in
  moby/buildkit#4022 and moby/buildkit#3217 ("Rootless does not
  start on GKE Autopilot" — GKE Autopilot enforces a security
  posture similar to PSA baseline).

Per the [PSA spec](https://kubernetes.io/docs/concepts/security/pod-security-standards/):
- Baseline forbids `seccompProfile.type: Unconfined`
- Baseline forbids `appArmorProfile.type: Unconfined`
- Baseline forbids adding capabilities outside the allow-list (which
  excludes `SYS_ADMIN`, required for `mount`/`unshare`)
- Baseline forbids `procMount: Unmasked`

Every in-cluster image builder hits at least one of these.

## Validation runs

The findings above were verified by running E2E with PSA enforcement
on:

- AKS PSA baseline workflow (`e2e-aks-psa-baseline.yaml`)
- EKS PSA baseline workflow (`e2e-eks-psa-baseline.yaml`) — run
  #27164309017 proved admission + daemon-start; run #27164822141
  confirmed the `--oci-worker-no-process-sandbox` flag is rootless-only
- k3d PSA baseline workflow (`e2e-k3d-psa-baseline.yaml`) — admission
  passed, daemon blocked by inner-Docker AppArmor

Full debug logs available in the demo repo's Actions history.

## Possible future paths

- **Localhost seccomp profile via DaemonSet.** PSA baseline accepts
  `seccompProfile.type: Localhost`, so in principle a custom seccomp
  profile that allows the syscalls BuildKit needs (mount, pivot_root,
  unshare with `CLONE_NEWUSER`, /proc operations) could work. Requires
  shipping a JSON profile to every node's
  `/var/lib/kubelet/seccomp/` directory via a DaemonSet, plus
  validating it doesn't expose more than the standard runtime default.
  Untested.
- **Out-of-cluster builds.** Move image building to an external
  CI/CD pipeline; have `Radius.Compute/containerImages` orchestrate
  remote builds rather than building in-cluster. Pursuing this in
  the longer term would sidestep the entire PSA-baseline problem.
- **K8s 1.36+ user namespaces (GA April 2026).** May shift upstream
  BuildKit's stance on requiring Unconfined seccomp. Worth a
  re-evaluation in 6-12 months once Linux distros and BuildKit
  catch up.
- **Direct sandboxed builders.** Newer experimental tools like
  `crane` (go-containerregistry) can construct images from layers
  without invoking the OCI runtime at all, sidestepping the entire
  seccomp/cap problem — but only support a narrow subset of
  Dockerfile features.

## References

- moby/buildkit#4022 — "Question: buildkit rootless + AppArmor on
  k8s- could Kubernetes UserNamespacesStatelessPodsSupport feature
  (v1.25 alpha) make this possible?" — confirms AppArmor is the
  fundamental blocker.
- moby/buildkit#3217 — "Rootless does not start on GKE Autopilot" —
  same constraint manifesting on Google's managed K8s.
- Kubernetes Pod Security Standards spec:
  https://kubernetes.io/docs/concepts/security/pod-security-standards/
- BuildKit rootless docs (lists every required `--security-opt`):
  https://github.com/moby/buildkit/blob/master/docs/rootless.md
- Kaniko archival notice (June 2025):
  https://github.com/GoogleContainerTools/kaniko

