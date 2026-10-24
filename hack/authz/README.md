# Manual authorization test kit

Scripts for checking Radius internal component authorization trust boundaries by hand on a local kind cluster. Reviewers use them for pull requests that add or change a trust boundary from the [internal component authorization design](../../eng/design-notes/security/2026-09-internal-component-authorization.md); the [checklist](#manual-verification-checklist) below lists each planned check. Automated coverage of the same boundaries lives in [`test/functional-portable/authz`](../../test/functional-portable/authz).

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/), running
- [kind](https://kind.sigs.k8s.io/) (`make install-kind` installs the pinned version)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/) (`make install-helm`)
- The `rad` CLI (see the [build docs](../../docs/contributing/contributing-code/contributing-code-building/README.md), or set `RAD` to a locally built binary)
- Go and GNU Make, to build the images ([prerequisites](../../docs/contributing/contributing-code/contributing-code-prerequisites/README.md))

## Scripts

Every script prints its options with `--help`. They target the kubeconfig context `kind-radius-authz` (set `AUTHZ_KUBE_CONTEXT` to use another) so they never act on a different cluster by accident.

| Script            | What it does                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
|-------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `up.sh [MODE]`    | Creates or reuses the kind cluster `radius-authz` with a local registry at `localhost:5001`, builds and pushes the Radius images from this checkout (`make docker-build-<image> docker-push-<image>`), and installs Radius from `deploy/Chart` with `rad install kubernetes --reinstall`. `MODE` is `off`, `dryRun` (default), or `enforce`. Prints the authz mode each component logged and the next steps.                                                                                                                                                                                                   |
| `rogue.sh`        | Manages a rogue workload pod ([`rogue-pod.yaml`](rogue-pod.yaml)) in an environment namespace (default `default`): `up`, `exec -- <request flags> URL`, `openssl -- <openssl args>`, and `down`. The pod has no service account token and uses the locally built rogue-client and a digest-pinned OpenSSL sidecar.                                                                                                                                                                                                                                                                                             |
| `as-component.sh` | `as-component.sh <component> -- <request flags> URL` copies the component's mTLS certificate, key, and CA from its Secret in `radius-system` into the rogue pod and runs rogue-client with `--cert`, `--key`, and `--cacert`, to act as a compromised component. Exits 3 and names the expected Secret when it does not exist.                                                                                                                                                                                                                                                                                 |
| `records.sh`      | Placeholder for showing UCP execution records. Prints `execution records are added in Stack B` and exits 0.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `would-deny.sh`   | Runs [`authz-would-deny-check.sh`](../../.github/scripts/authz-would-deny-check.sh) against the kit cluster and fails if any Radius pod logged a dry-run would-deny line. When `up.sh` installed the cluster in `dryRun` mode, it adds `--require-dry-run`, so it also fails unless `ucp`, `applications-rp`, `dynamic-rp`, and `controller` all started in `dryRun`. Unlike `make authz-would-deny-check`, which always passes `--require-dry-run` and uses the current context, it targets the kit context and only requires `dryRun` for a `dryRun` install; pass `--require-dry-run` to require it anyway. |

Shared settings and helpers live in `lib.sh`. Run `make test-authz-kit` after changing the kit; it runs `bash -n`, each script's `--help`, and tests for the helpers with a stubbed `kubectl`. Run `make lint-shell` separately for ShellCheck static analysis.

The kit uses the same shell-free `rogue-client` as automated `test/rogue` probes. `up.sh` builds and pushes it, then records its resolved image in the `radius-authz-kit` ConfigMap in `radius-system`. `rogue.sh` renders that image into the pod manifest (override with `AUTHZ_ROGUE_IMAGE` if needed). If a pod uses an older image, run `rogue.sh down` before retrying. The digest-pinned OpenSSL sidecar provides diagnostics and shell-based certificate streaming; both containers run as UID 65532 so private certificate files remain readable by the client without broadening file permissions.

`rogue.sh exec -- [flags] URL` runs `/rogue-client request`. Supported flags are `-X`, repeatable `-H`, `--data`, `--cert`, `--key`, `--cacert`, `-k`, and `--max-time` (a duration such as `30s`); put all flags before the URL. Output is the response body followed by `radius-rogue-status:<HTTP code>`. Exit 0 means an HTTP response was received, including 4xx/5xx; exit 1 means a transport failure, with status 0; exit 2 means invalid arguments. Other flags such as `-s`, `-o`, and `-w` are not supported.

`would-deny.sh` fails with exit code 2 if it cannot read the Radius namespace, even when scanning saved logs with `--logs-dir`. A successful namespace read with no mode label retains the unknown-mode warning; pass `--require-dry-run` to require live startup verification in that case. Set `AUTHZ_RADIUS_NAMESPACE` to select another namespace for both the mode label and logs. The wrapper rejects `--namespace` overrides so these cannot refer to different installations.

### Component certificates

`as-component.sh` reads the Secret named by `AUTHZ_COMPONENT_SECRET_FORMAT` in `lib.sh`, where `%s` is the component name (default `%s-mtls`, for example `dynamic-rp-mtls`), with the keys `tls.crt`, `tls.key`, and `ca.crt`. Stack A adds these Secrets and sets the final naming in that one variable. Until then the script reports which Secret is missing.

### Execution records

Once Stack B adds execution records, `records.sh` lists them, one line per record (`ID STATUS DEPLOYMENT CREATED EXPIRES TARGETS`), and `records.sh <id>` shows a record's status (active, closed, or expired), parent record, approved actions and targets, and its operations with their assigned component and `inputHash`.

## Quick start

```bash
hack/authz/up.sh dryRun

# Call applications-rp from a rogue workload. Before Stack A this returns 200.
hack/authz/rogue.sh exec -- \
  'http://applications-rp.radius-system:5443/planes/radius/local/providers/Applications.Core/operations?api-version=2023-10-01-preview'

# The response ends with radius-rogue-status:200.
hack/authz/would-deny.sh
```

Rerun `up.sh` with another mode to switch modes; add `--skip-build` to reuse the last images. To remove everything:

```bash
kind delete cluster --name radius-authz && docker rm --force radius-authz-registry
```

## Manual verification checklist

Stack labels and check IDs are implementation roadmap identifiers used by this kit, not the three stages in the design's [development plan](../../eng/design-notes/security/2026-09-internal-component-authorization.md#development-plan):

- **Stack 0**: foundation (mode configuration and test tooling).
- **Stack A**: component mTLS and authentication boundaries.
- **Stack B**: execution records, grants, and input binding.
- **Stack C**: admission binding.
- **Stack D**: broker and target access.
- **Stack E**: admission safeguards.

The prefix identifies the stack and the numeric suffix identifies a check within that roadmap; for example, `A4` belongs to Stack A and `B5` to Stack B. `0.1` is a foundation check. The table describes each check's goal; these identifiers do not imply a one-to-one mapping to the design stages.

`APPS_RP` below is `applications-rp.radius-system.svc:5443`, and `OPS` is the harmless route `/planes/radius/local/providers/Applications.Core/operations?api-version=2023-10-01-preview`. Commands for checks that are not available yet show the intended flow; the pull request that adds the boundary updates them if the details change.

Run these assignments in the shell where you will run the checklist commands:

```bash
APPS_RP="applications-rp.${AUTHZ_RADIUS_NAMESPACE:-radius-system}:5443"
OPS='/planes/radius/local/providers/Applications.Core/operations?api-version=2023-10-01-preview'
```

For A4, applications-rp and dynamic-rp accept both plain HTTP and HTTPS on the same port in `dryRun` (logging a would-deny for each connection without a valid client certificate) and only mTLS in `enforce`. Run the HTTPS checks with `hack/authz/up.sh enforce`. The HTTPS negative test deliberately uses `-k` to skip server-certificate verification on this local test cluster, so an untrusted server CA cannot be mistaken for rejection of a missing client certificate. It supplies no client certificate; confirm the TLS rejection on stderr, exit 1, `radius-rogue-status:0`, and `PeerCertificateInvalid` in the applications-rp logs, not an HTTP error body. Plain HTTP against a Go TLS listener returns HTTP 400 and does not test the mTLS handshake. The `as-component.sh` checks verify the server certificate using the component Secret's CA.

| Check    | Available | Command                                                                                                                                                                                                                                                                         | Expected result                                                                                                                                                                                                                   |
|----------|-----------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 0.1      | Now       | `hack/authz/up.sh dryRun`, then read the printed modes, or `kubectl logs -n radius-system deploy/<component> \| grep 'authz mode='` for `ucp`, `applications-rp`, `dynamic-rp`, and `controller`                                                                                | All match: `authz mode=dryRun`                                                                                                                                                                                                    |
| A4       | Now       | Without a client certificate: `hack/authz/rogue.sh exec -- -k "https://$APPS_RP$OPS"`. As dynamic-rp or UCP: `hack/authz/as-component.sh dynamic-rp -- "https://$APPS_RP$OPS"`, then the same with `ucp`. In `dryRun`, also `hack/authz/rogue.sh exec -- "http://$APPS_RP$OPS"` | `enforce`: TLS rejection, no HTTP response, and `PeerCertificateInvalid` logged; as dynamic-rp: `AuthorizationFailed` (403); as UCP: accepted. `dryRun`: every request returns HTTP 200 and the failures are logged as would-deny |
| A6       | Stack A   | `hack/authz/rogue.sh exec -- -k -H 'x-remote-user: admin' 'https://ucp.radius-system/apis/api.ucp.dev/v1alpha3/planes?api-version=2023-10-01-preview'`                                                                                                                          | `InvalidAuthenticationInfo`                                                                                                                                                                                                       |
| B3       | Stack B   | Deploy an app, find its record with `hack/authz/records.sh`, then PUT with `hack/authz/as-component.sh bicep-de -- -X PUT ...` to a resource outside the record's targets, after the record closes, and as another component                                                    | `GrantScopeExceeded` / `ExecutionRecordNotActive` / rejected                                                                                                                                                                      |
| B5       | Stack B   | `hack/authz/as-component.sh ucp -- -X PUT --data '<tampered body>' "https://$APPS_RP/<resource>"` for an operation whose recorded `inputHash` covers a different body                                                                                                           | `OperationInputMismatch`                                                                                                                                                                                                          |
| C3       | Stack C   | Apply a `DeploymentTemplate` in namespace `team-a` that targets resource group `rg-b`, then `kubectl get deploymenttemplate -n team-a -o yaml` and `rad group list`                                                                                                             | Rejected, a status condition explains why, no resource group is created                                                                                                                                                           |
| D2–D4    | Stack D   | `hack/authz/as-component.sh dynamic-rp -- -X PUT ...` to the data-access service for a deployment that is not assigned to dynamic-rp                                                                                                                                            | Denied                                                                                                                                                                                                                            |
| D5       | Stack D   | `kubectl auth can-i create resources.ucp.dev -n radius-system --as system:serviceaccount:radius-system:dynamic-rp`                                                                                                                                                              | `no`                                                                                                                                                                                                                              |
| D7       | Stack D   | Deploy an application template that creates a `ClusterRole`, then a recipe that creates one                                                                                                                                                                                     | Template: denied. Recipe: allowed                                                                                                                                                                                                 |
| D8       | Stack D   | `kubectl get deploy -n radius-system -o yaml \| grep -n kubeconfig` and `kubectl auth can-i --list --as <kubeconfig identity>` on the target cluster                                                                                                                            | As stated: only the broker mounts the target kubeconfig; the kubeconfig identity can only create `serviceaccounts/token`                                                                                                          |
| E1/E2/E3 | Stack E   | Deploy an application with a privileged container; one that uses a control-plane service account or Secret as its template identity; and an environment whose namespace is `radius-system`                                                                                      | Rejected / `AdmissionPolicyDenied` / rejected                                                                                                                                                                                     |

## Troubleshooting

| Symptom                                                     | Fix                                                                                                                                                                                                            |
|-------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `the Docker daemon is not reachable`                        | Start Docker and rerun.                                                                                                                                                                                        |
| Port 5001 is already in use                                 | Set `AUTHZ_REGISTRY_PORT` to a free port. Use the same value for later runs.                                                                                                                                   |
| Radius pods crash with `exec format error` on an arm64 host | `make docker-build` builds `linux/amd64` images, which need Docker's amd64 emulation (on by default in Docker Desktop). Enable it, or build multi-arch images as described in the `radius-build-images` skill. |
| `kubeconfig context 'kind-radius-authz' not found`          | Run `up.sh`, set `AUTHZ_KUBE_CONTEXT`, or pass `--current-context` to `would-deny.sh`.                                                                                                                         |
| `Component certificate Secret not found`                    | Expected before Stack A. Afterwards, check `AUTHZ_COMPONENT_SECRET_FORMAT` in `lib.sh` against the Secrets in `radius-system`.                                                                                 |
