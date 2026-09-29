# Repo Radius GHCR state end-to-end test

## Purpose

This page describes the standalone [`repo-radius-state-e2e.yaml`](../../../../.github/workflows/repo-radius-state-e2e.yaml) workflow, a daily end-to-end test of the OCI state archive, and how to provision, run, and troubleshoot it. It is for maintainers who own the scheduled run and for contributors changing the OCI state archive or its workflow. For the OCI state archive itself, see the [durable state archive architecture](../../../architecture/state-archive.md).

## How the test works

[`repo-radius-state-e2e.yaml`](../../../../.github/workflows/repo-radius-state-e2e.yaml) deploys an application through one ephemeral Radius control plane to a separate persistent k3d target cluster, saves Radius state to a private GHCR package, replaces the control plane, restores the saved state, and updates the existing target-cluster workload.

The scheduled run uses `ghcr.io/radius-project/radius-repo-state-e2e`. This package must be precreated as private and linked to `radius-project/radius`; a workflow in the public repository would otherwise create a public package, which Radius refuses to use for state.

## Provision the state package

Provision or verify the package with a GitHub CLI credential that has `write:packages`:

```bash
$ make install-oras
$ export PATH="$HOME/.local/bin:$PATH"
$ ./.github/scripts/precreate-repo-radius-state-package.sh \
    --package ghcr.io/radius-project/radius-repo-state-e2e \
    --source-repository https://github.com/radius-project/radius
```

The script is idempotent. It creates or verifies a harmless `bootstrap` version, private/internal visibility, and repository linkage. It never changes a public package to private.

## Run the test

The workflow runs daily at 04:17 UTC from the default branch. For manual testing, dispatch another ref and optionally select a different precreated package under that repository owner:

```bash
$ gh workflow run repo-radius-state-e2e.yaml \
    --ref <branch> \
    -f state_package=<package-name>
```

## Diagnostics and failure handling

Each lifecycle phase is a separate workflow step. On failure, inspect the failed step first, then download the `repo-radius-state-e2e-diagnostics` artifact. A separate post-lifecycle job retries run-specific state cleanup even if the main job times out. Scheduled failures in the upstream repository also create an issue labeled `test-failure`; manual and fork runs do not. Successful runs record the saved/restored digest in the job summary; the private package and bootstrap version remain.

## Related docs

- [Durable state archive](../../../architecture/state-archive.md) — the OCI state archive this test exercises.
- [Running functional tests](./running-functional-tests.md) — the broader end-to-end test tier.
- [Contributing to GitHub Actions workflows](../contributing-code-github-workflows/README.md) — how to change the automation under `.github/workflows/`.
