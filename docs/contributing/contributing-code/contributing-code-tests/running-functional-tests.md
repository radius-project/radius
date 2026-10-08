# Run functional tests locally

## Purpose

Use this guide to build Radius from your checkout and run the non-cloud functional tests in local k3d clusters. The test runner creates its own Kubernetes clusters, Radius configuration, container registry, and test fixtures. It does not use your current Kubernetes context or Radius workspace.

You do not need a cloud account, GitHub personal access token, or external container registry.

## Prerequisites

Use Debian/Ubuntu Linux (including Windows with WSL 2) or macOS, on an `amd64` or `arm64` processor. Linux certificate setup uses `update-ca-certificates` and `/usr/local/share/ca-certificates`; other Linux distributions are not supported by this runner.

The runner builds `rad` for the host and Linux service images for the Docker daemon's architecture. An arm64 Docker daemon must also support amd64 container emulation because the published dashboard and some test fixtures are amd64-only. Setup checks this before building images or changing host trust.

Install these tools and make sure each command is available on `PATH`:

- Docker
- Git
- Go
- A C compiler for the Go tests (GCC on Linux or Xcode Command Line Tools on macOS)
- GNU Make
- k3d
- kubectl
- Helm
- OpenSSL 1.1.1 or later
- jq
- Python 3
- envsubst (provided by gettext)
- curl 7.71.0 or later
- Flux CLI 2.6.4
- [yq v4](https://github.com/mikefarah/yq) for Bicep type generation
- Node.js and Corepack (see the [code-generation prerequisites](../contributing-code-prerequisites/README.md#install-code-generation-tools))

Start Docker before running the tests. The test runner installs the repository-pinned Bicep and Dapr CLIs. It adds the temporary registry CA to the host trust store without overriding `SSL_CERT_FILE`, preserving public HTTPS trust and any caller-supplied CA bundle.

The test runner also requires:

- `sudo` access to trust a temporary registry certificate and add `radius-local-registry` to `/etc/hosts`.
- Free TCP ports 5443, 6550-6556, and 30080.
- Sufficient CPU and memory to run up to seven k3d clusters concurrently. Use the selected-group workflow below on resource-constrained machines.
- On Linux, at least 1024 inotify instances per user. Check the current value with `sysctl fs.inotify.max_user_instances`.

### Windows and WSL

Run the workflow inside a Debian/Ubuntu WSL 2 distribution, not PowerShell, Command Prompt, or Git Bash. Install Linux versions of Go and the other tools for the distribution's architecture (`amd64` or `arm64`); do not use Windows executables from the shared Windows `PATH`.

Run Docker Engine inside the WSL distribution. The runner changes the distribution's Linux certificate trust and hosts file; it does not configure Windows or a separate Docker Desktop VM's certificate trust. Use a checkout on the WSL Linux filesystem for better build performance.

### Arm64 container emulation

On an arm64 Docker daemon, this compatibility check must succeed:

```bash
docker run --rm --platform linux/amd64 alpine:3.24.2 /bin/true
```

For an arm64 Linux Docker Engine, an administrator can register amd64 QEMU emulation with the following command. It changes the Docker host's binary-format handlers and requires a trusted daemon:

```bash
docker run --privileged --rm \
  tonistiigi/binfmt@sha256:400a4873b838d1b89194d982c45e5fb3cda4593fbfd7e08a02e76b03b21166f0 \
  --install amd64
```

Rerun the compatibility check afterward. Registration may need repeating after the Docker host or WSL VM restarts. The runner does not register emulators or change these host settings automatically.

### macOS tool selection

Use Docker Desktop with Linux containers. On Apple Silicon, enable support for running `linux/amd64` containers and verify it with the compatibility check above. Radius service images are built natively for the Docker daemon, while `rad` is built for your Mac's native architecture.

If you use Homebrew, make GNU Make, OpenSSL, curl, and envsubst available under the command names used by the runner:

```bash
brew install make openssl@3 curl gettext
export PATH="$(brew --prefix make)/libexec/gnubin:$(brew --prefix openssl@3)/bin:$(brew --prefix curl)/bin:$(brew --prefix gettext)/bin:$PATH"
```

Check `make --version`, `openssl version`, `curl --version`, and `envsubst --version` before continuing. Apple's bundled OpenSSL-compatible tool may not support the certificate-generation flags used by this workflow.

## Steps

### 1. Generate the Radius Bicep extension

From the repository root, run:

```bash
make generate-bicep-types
```

This command creates the Bicep extension used to compile the test templates against the resource types in your checkout.

### 2. Run all local non-cloud functional tests

Run:

```bash
make test-functional-local-noncloud
```

This target:

1. Builds the `rad` CLI for your host and Radius Linux container images for the Docker daemon's architecture (`amd64` or `arm64`) from your checkout.
2. Creates a local container registry and a k3d cluster.
3. Installs Radius and deploys `test/infra/radius/legacy-test-environment.bicep`, selecting its full `Applications.Core` environment ID directly in the isolated workspace configuration. This keeps legacy tests independent of the `Radius.Core`-only `rad env` commands. Then installs Flux, Git HTTP, and Dapr concurrently with recipe publishing. Publishes Bicep recipes with at most four workers; other callers of the publisher remain serial unless they set `BICEP_RECIPE_PUBLISH_PARALLELISM` to 2, 3, or 4. Waits for every fixture to be ready, restores and verifies the local Bicep extensions, and copies the validated cache into independent worker caches before parallel template compilation.
4. Runs the UCP group first, preserving the custom-provider discovery coverage in Core RP, then runs the other seven standard groups through three concurrent, sequential worker lanes. Each group has its own HOME, Radius configuration, Bicep extension cache, and container-log directory. Core RP retains ten parallel test slots; other standard groups use five.
5. Concurrently runs Dynamic RP, database, multicluster, statestore, and upgrade groups in separate clusters. Dynamic RP uses five parallel test slots and serializes shared provider/type registration; the other isolated groups retain one slot each. Dynamic RP's custom provider metadata is kept out of the standard cluster so it cannot interfere with provider-discovery tests.

Each setup refreshes the retained samples checkout from its `edge` branch and archives previous test reports and container logs before starting a new run. The full lifecycle controls concurrency independently of Make's parallel settings. Manually selected groups still run sequentially, even when your Make settings request parallel execution. All deletion and post-delete checks remain enabled.

Do not start another local functional test run at the same time. Fixture publication and test execution each hold the shared Bicep fixture lock and temporarily configure Bicep to use the extensions published from your checkout. Both phases drain their workers before restoring the original files and releasing the lock, including on failure or interruption.

### 3. Clean up

After reviewing the results, remove the clusters, registry, trusted certificate, and `/etc/hosts` entry:

```bash
make test-functional-local-noncloud-cleanup
```

The command retains logs and test configuration under `dist/local-functional/`. `make test-functional-noncloud-cleanup` is an alias for the same cleanup.

### Run fewer tests

Use this workflow only when you do not need the complete test suite.

First generate the Bicep extension and create the standard test environment:

```bash
make generate-bicep-types
bash build/scripts/local-functional.sh setup
```

Run one standard test group by passing its Make target:

```bash
bash build/scripts/local-functional.sh test test-functional-corerp-noncloud
```

Run one test by passing Go test options through Make:

```bash
bash build/scripts/local-functional.sh test test-functional-cli-noncloud \
  GOTEST_OPTS='-count=1 -run Test_CLI_version'
```

Run one group that needs a separate cluster:

```bash
bash build/scripts/local-functional.sh isolated database
```

Replace `database` with `multicluster`, `statestore`, `upgrade`, or `dynamicrp` as needed. Manually selected isolated groups are serialized; do not launch separate runner commands concurrently.

## Verification

The complete test run succeeds when `make test-functional-local-noncloud` exits with status 0.

Test output and per-group gotestsum JSON files are stored under `dist/local-functional/results/`. During the full lifecycle, each worker writes a separate `.log` file there; the runner prints these logs after the workers finish and fails if any worker failed. Container logs are stored under `dist/local-functional/container_logs/`, with separate subdirectories for full-lifecycle groups.

The results and container-log directories contain diagnostics only for the current setup. Previous reports and logs are retained together under `dist/local-functional/results-history/run.*/`, in `results/` and `container_logs/` respectively; do not use those archived files to determine whether the current run passed.

Before cleanup, confirm that Radius is running in the standard test cluster:

```bash
bash build/scripts/local-functional.sh exec kubectl get pods -n radius-system
```

After cleanup, the command prints:

```text
Infrastructure removed; logs and isolated CLI configuration remain in dist/local-functional.
```

For manually selected groups, each package prints `ok` or a successful `gotestsum` summary. A successful end-to-end group creates its expected Radius resources, verifies their state, and removes them according to the selected cleanup mode.

Run `make test-local-functional-runner` to check the runner's amd64/arm64 platform and emulation preflight, image architecture selection, bounded Gateway API discovery readiness, explicit legacy environment deployment and selection, fixture locking/restoration, diagnostics rotation, bounded full-lifecycle concurrency, worker failure propagation, serial selected-group execution, and cleanup using mocked infrastructure commands. These regression checks also run as part of `make test` and do not require Docker, Kubernetes, or sudo access.

Run `make test-docker-build` to verify the default amd64 and explicit amd64/arm64 Make image recipes without Docker or Go builds. These checks also run as part of `make test`.

Run `make test-publish-recipes` to check serial and bounded concurrent recipe publishing, module-file exclusion, failure propagation, and worker draining without a registry. These checks also run as part of `make test`.

## Troubleshooting

### Arm64 setup fails its amd64 compatibility check

Read the Docker error printed before the runner's emulation message. Resolve image-pull or daemon connectivity errors first. An `exec format error` means the Docker host cannot execute the amd64 probe; enable emulation as described in [Arm64 container emulation](#arm64-container-emulation), then retry. Do not just set `DOCKER_DEFAULT_PLATFORM`: that selects images but does not install an emulator.

### Existing test infrastructure

Setup stops instead of replacing an existing test cluster or registry. Remove the previous test infrastructure, then start the test run again:

```bash
make test-functional-local-noncloud-cleanup
make test-functional-local-noncloud
```

### Linux inotify limit is too low

If setup reports that `fs.inotify.max_user_instances` is below 1024, raise it for the current boot:

```bash
sudo sysctl -w fs.inotify.max_user_instances=1024
```

An exhausted inotify limit can leave the Kubernetes API server running without a registered node.

### An isolated test group failed

The runner keeps a failed isolated cluster for inspection. Select the failed group and run a command against its cluster:

```bash
LOCAL_FUNCTIONAL_SUITE=database \
  bash build/scripts/local-functional.sh exec kubectl get pods -A
```

Replace `database` with `multicluster`, `statestore`, `upgrade`, or `dynamicrp`.

Retry the matching test target against the retained cluster:

```bash
LOCAL_FUNCTIONAL_SUITE=database \
  bash build/scripts/local-functional.sh test test-functional-database-noncloud
```

### A complete test run failed

Review `dist/local-functional/results/` and `dist/local-functional/container_logs/`. Run cleanup before starting another complete test run because failed tests can leave resources and provider registrations behind.

### Gateway API discovery is not ready after installation

Newly installed Gateway API resources may not appear immediately in Kubernetes API discovery. The runner refreshes discovery and waits up to three minutes for both GatewayClass and Gateway resource types before checking the Contour gateway. If that wait fails, inspect the Gateway API CRDs and the Radius installation in the retained cluster; authentication and API connection errors fail immediately rather than being retried as missing resources.

### Bicep reports that a local extension has not been restored

`BCP190` for `br:localhost:5443/radius:<tag>` or `br:localhost:5443/testresources:<tag>` is a local extension-cache error, not a deployed-resource failure. The runner restores both extensions serially in the selected suite's isolated HOME and verifies a template with `--no-restore` before launching the tests. This avoids relying on concurrent builds to populate a shared cold cache.

If this preparation fails, inspect the restore/build error and confirm that the setup completed and the local registry is running. Tests do not start after a failed extension preparation. Keep the same checkout commit between setup and test commands, because the published extension tags are derived from that commit.

### An older runner left a fixture lock

If the runner reports `Another local functional test run owns the Bicep fixtures`, first confirm that no local functional test run is still active. An older version of the runner used an empty file at `dist/local-functional/test.lock`; the current version uses a directory containing its owner's process ID.

Only when no test run is active, remove the legacy file if it exists:

```bash
if test -f dist/local-functional/test.lock; then
  rm dist/local-functional/test.lock
fi
```

Do not remove an active runner's lock directory. Clean up and restart the complete run as described above.

## Functional test reference

The local workflow above is the recommended path for running all non-cloud functional tests. The following reference material covers manual environments, individual Make targets, cleanup behavior, and CI.

### Existing environment or cloud tests

To run functional tests against an existing environment:

1. Place `rad` on your path.
2. Download Bicep with `rad bicep download`.
3. Complete the [local development environment setup](../contributing-code-debugging/radius-os-processes-debugging.md).
4. For tests that publish packages to GHCR, create a GitHub personal access token and log in to your organization's registry:

   ```bash
   export CR_PAT=<your_pat>
   echo "${CR_PAT}" | docker login ghcr.io -u <your_username> --password-stdin
   ```

5. Publish the Bicep test recipes:

   ```bash
   BICEP_RECIPE_REGISTRY=<registry-name> make publish-test-bicep-recipes
   ```

6. Publish the Terraform test recipes:

   ```bash
   make publish-test-terraform-recipes
   ```

   Set `TF_RECIPE_MODULE_SERVER_URL` to the URL that test deployments can reach. CI and the local runner use `http://tf-module-server.radius-test-tf-module-server.svc.cluster.local`; direct local tests default to `http://localhost:8999`.

7. Make packages published for the tests publicly readable.

The tests assume the current Kubernetes namespace is `default`.

### Local Bicep registry trust

Bicep rejects registry hosts outside its trusted allowlist. For the secure local registry used by `functional-test-noncloud.yaml`, trust `localhost` and `radius-registry` only for the commands that need them:

```bash
BICEP_TRUSTED_REGISTRIES=localhost,radius-registry \
  BICEP_RECIPE_REGISTRY=localhost:5000 make publish-test-bicep-recipes
BICEP_TRUSTED_REGISTRIES=localhost,radius-registry \
  BICEP_RECIPE_REGISTRY=radius-registry:5000 make test-functional-cli-noncloud
```

Use bare hostnames without `br:`, ports, or paths. Registry trust does not bypass TLS or certificate verification.

### Run tests against an existing environment

To provision Azure resources and run the cloud-capable Core RP, Messaging RP, Dapr RP, and Datastores RP groups:

```bash
./test/executeFunctionalTest.sh <resource-group-name>
```

To run a group directly, call its Make target:

```bash
make test-functional-corerp-noncloud
make test-functional-all-noncloud
```

The supported groups, including `authz`, and variants are defined in [`build/test.mk`](../../../../build/test.mk).

The `authz` group (`make test-functional-authz-noncloud`) covers internal component authorization. Its smoke test reads the installed Helm values and checks that every Radius component logs the matching `authz mode=<mode>`, so it passes on any install. The `authz-noncloud` CI leg installs Radius with `--set global.rbac.dryRun=true` so the test checks a non-default mode; to reproduce that locally, run `rad install kubernetes --set global.rbac.dryRun=true` before the tests.


### Run a special test group

`make test-functional-all-noncloud` excludes these isolated groups:

| Target                                       | Requirements and behavior                                                                                                                          |
|----------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| `make test-functional-multicluster-noncloud` | Requires a second Kubernetes cluster, a target-cluster Secret mounted into Radius, and `RADIUS_TEST_EXTERNAL_KUBECONFIG` for the test process.     |
| `make test-functional-database-noncloud`     | Requires Radius installed with `--set database.enabled=true` (PostgreSQL-backed control plane) instead of the default Kubernetes API server store. |
| `make test-functional-statestore-noncloud`   | Destructive lifecycle test that installs, purges, and reinstalls Radius. Run it only on a dedicated cluster.                                       |
| `make test-functional-upgrade-noncloud`      | Exercises the Radius upgrade path, including `rad upgrade kubernetes`, and performs its own install/upgrade lifecycle.                             |

The local runner provisions dedicated clusters for these groups. When running them manually, do not use a shared development cluster.

For database tests, install Radius into a fresh cluster with `--set database.enabled=true`.

For multicluster tests, create a `target-kubeconfig` Secret in `radius-system`, install Radius with `global.targetCluster.enabled=true`, and set `RADIUS_TEST_EXTERNAL_KUBECONFIG` to the host-side kubeconfig.

### Configure test execution

| Variable                          | Purpose                                                                                                                                                                |
|-----------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `TEST_TIMEOUT`                    | Overrides the Go test timeout. The default in `build/test.mk` is `1h`.                                                                                                 |
| `DE_IMAGE`, `DE_TAG`              | Select the Deployment Engine repository and tag for every test-owned upgrade-test install and upgrade. Set both together or leave both empty to retain chart defaults. |
| `RADIUS_TEST_EXTERNAL_KUBECONFIG` | Points multicluster tests at the external workload cluster.                                                                                                            |
| `TF_RECIPE_MODULE_SERVER_URL`     | Overrides the Terraform recipe module server URL.                                                                                                                      |
| `RADIUS_TEST_FAST_CLEANUP`        | Selects standard or fast cleanup.                                                                                                                                      |

Set `RADIUS_REGISTRY_CERT_FILE` when Radius images come from a registry that uses a private CA.

### Control test cleanup

- **Standard cleanup** is the default for local development. It waits for resource deletion and surfaces failures.
- **Fast cleanup** is the default in CI. It starts deletion without waiting and relies on removal of the per-run cluster and cloud resource group.

Set `RADIUS_TEST_FAST_CLEANUP=true` only for disposable environments. Do not use fast cleanup to validate teardown behavior because it intentionally ignores deletion errors.

### Resource deletion order

Tests delete applications, application-scoped resources, environments, and recipe packs in that order. Recipe-backed resources must be deleted before their environment or recipe pack because deletion loads environment configuration.

### Run or debug tests in VS Code

Set `go.testTimeout` to `60m` and add `-v` to `go.testFlags`. Set `RAD_PATH` in `go.testEnvVars` when CodeLens cannot resolve the locally built CLI.

### How the tests run in CI

The `functional-test-noncloud.yaml` and `functional-test-cloud.yaml` workflows build Radius, publish test assets, create isolated infrastructure, run each selected group, capture diagnostics, and remove the test infrastructure.

The scheduled purge workflows remove cloud resources left by cancelled or timed-out runs. The `long-running-azure.yaml` workflow uses published Radius artifacts while running diagnostics from the `main` workflow checkout.

Download the `all_container_logs` artifact to investigate a run. Treat downloaded logs as potentially sensitive.

Run `make test-cluster-diagnostics` to test the diagnostic scripts without accessing a cluster.

### Diagnose Terraform provider failures

Failed `corerp-cloud` jobs print a selected Terraform diagnostic summary in the **Collect cluster diagnostics** step. The same summary is saved as `corerp-cloud-terraform-diagnostics.log` in the `corerp-cloud_container_logs` artifact. Collection is limited to 120 seconds, with a 15-second limit per request, and does not replace the test result.

The summary includes runner memory, node capacity and pressure, warning event identities, and the state of `applications-rp` and `dynamic-rp` containers. It includes current and previous termination reasons, exit codes, signals, timestamps, memory requests and limits. For running containers, it reads available cgroup memory usage, peak, limit, OOM counters, CPU counters, and task counts. These are post-test snapshots and container-lifetime counters, not measurements of an individual provider process. A provider can be killed without restarting its parent container; check cgroup OOM counters as well as pod termination state. Unavailable cgroup files are omitted. If no counters can be read, or a container or request is inaccessible, collection reports the failure.

Provider error summaries count JSON log records that match schema errors, provider-start timeouts, or a plugin that did not respond. They read at most the last 40 minutes, 2000 lines, and 1 MiB per container, including the previous container when it has restarted. Counts are not failed-operation counts; one error can appear in several records. Non-JSON records are counted but their contents are not printed. The summary excludes raw provider logs, event and termination messages, pod environment values, and process command lines. This collection does not enable Terraform trace logging.

To collect the same summary locally, use a Linux host with Bash, `kubectl`, `jq`, GNU coreutils (`timeout`), procps (`free`), and access to your selected cluster. Run `bash .github/scripts/collect-terraform-diagnostics.sh` on that host. The collector does not support native macOS execution. Run `make test-cluster-diagnostics` to check the collector with stub commands; it does not require a cluster.

### Diagnose long-running Azure test failures

The scheduled `long-running-azure.yaml` workflow uses a published release for the CLI, control plane, and test source. Releases do not publish test images, so the workflow builds the Magpie test image from that test source and pushes it to the functional-test registry under a run-specific `test-` tag. Its diagnostics scripts run from the workflow checkout on `main`, so diagnostic improvements do not require a new Radius release.

The LRT setup script [`.github/scripts/manage-radius-installation.sh`](../../../../.github/scripts/manage-radius-installation.sh) defines the required Helm overrides in `REQUIRED_CHART_VALUES`, shared by fresh installs, upgrades, and existing same-version IRSA reconciliation. Keep LRT-specific configuration in that array rather than changing global chart defaults or using `kubectl set resources`, which can cause field-ownership conflicts on later upgrades.

Download the `all_container_logs` artifact from a successful or failed run. Alongside the released test harness's logs, it contains:

- `all-tests-pod-states.log`: final pod, node, and event descriptions.
- `all-diagnostics/window-start.txt`: the UTC start of the test command, used to filter the final container log sweep.
- `all-diagnostics/events-*.log`: timestamped event JSON and `radius-system` pod summaries sampled throughout the test command, with a 60-second delay between samples. These preserve events that may expire before the final snapshot.
- `all-diagnostics/radius-system/containers.tsv`: pod UIDs, restart counts, images, and separate current/previous container identities and start/finish timestamps, without pod environment values. Missing fields are `-`; a running container has no current finish time. The column order is recorded in `collection.log`.
- `all-diagnostics/radius-system/*.current.log` and `*.previous.log`: timestamped retained container logs from the test window. Previous logs are requested for containers reporting a restart.
- `all-diagnostics/collection.log` and `sampler.log`: requested windows, collection times, command outcomes, capture failures, and limit notices.

The sampler stops with the wrapped command, including on cancellation, without replacing its exit status. A 130-minute safety ceiling exceeds the workflow's existing 120-minute test-step timeout. Sampling is capped at 20 MiB. The final container log sweep has a 120-second budget, a 10 MiB cap per capture, and a 100 MiB combined cap; individual requests are bounded to 15 seconds plus a two-second termination grace period. Final diagnostic collection has a five-minute workflow timeout and runs before artifact upload and cluster cleanup.

Container logs cannot recover deleted pods or rotated history, and `--previous` covers at most one restart of a surviving container. An old OOM termination is not evidence of an OOM during this run; compare its timestamp with `window-start.txt`. A quiet log interval alone does not prove rotation or a stalled collector. `FAILED` entries, output caps, cancellation, or a collection timeout can leave incomplete captures. Use the cluster's existing Azure monitoring for historical CPU, memory, throttling, and logs no longer retained by Kubernetes. Treat downloaded logs as potentially sensitive.

To collect the same diagnostics locally, use Bash, `kubectl`, `jq`, and GNU coreutils (`timeout`) with your selected cluster. Use an absolute output path and a fresh directory for each run:

```bash
export RADIUS_CONTAINER_LOG_PATH="$PWD/dist/lrt-diagnostics-local"
export DIAGNOSTICS_NAME=all
bash .github/scripts/run-with-cluster-diagnostics.sh \
  make test-functional-corerp-noncloud
# Run this even if the test command fails.
DIAGNOSTICS_LOG_NAMESPACES=radius-system \
  bash .github/scripts/collect-cluster-diagnostics.sh
```

`DIAGNOSTICS_SAMPLE_INTERVAL` overrides the sampling delay in seconds. `DIAGNOSTICS_SINCE_TIME` overrides the saved window with a UTC timestamp such as `2026-09-17T10:00:00Z`; without either window, the collector records that container logs were skipped rather than fetching unbounded history. `DIAGNOSTICS_COLLECTION_SECONDS` overrides the container sweep's time budget. Other functional workflows retain their existing snapshot behavior unless they opt in with `DIAGNOSTICS_LOG_NAMESPACES`.

Run `make test-cluster-diagnostics` to exercise collection, limits, failures, command status preservation, and sampler cancellation with a stubbed `kubectl`; it does not access a cluster. This optional target requires `jq` and GNU coreutils (`timeout`). The Ubuntu unit-test CI job invokes it separately; it is not a prerequisite of `make test`, so the standard local unit-test command does not acquire these additional tool requirements.

### Cloud credentials in CI (federated identity)

The cloud CI workflows - `functional-test-cloud.yaml`, the long-running test `long-running-azure.yaml` ("LRT"), and the two scheduled purge jobs - authenticate to Azure and AWS with **federated identity only**. No static cloud secrets (service-principal passwords or AWS access keys) are stored in GitHub; every credential is a short-lived token minted from an OIDC trust. Two distinct trusts are in play:

- **Runner -> cloud.** The GitHub Actions runner gets tokens from GitHub's OIDC provider (`token.actions.githubusercontent.com`), and Azure CLI / `aws-actions/configure-aws-credentials` exchange them for short-lived credentials that the test and purge code use to verify and delete cloud resources. Every such job sets `permissions: id-token: write`. The cloud functional workflow uses [`.github/scripts/azure-oidc-login.mjs`](../../../../.github/scripts/azure-oidc-login.mjs) for Azure login. Both jobs load this script in a separate checkout from `github.repository` at `github.sha`, so older PR branches do not need to contain it. Builds and tests still use the PR checkout. It retries only GitHub token request timeouts, selected connection errors, and HTTP 408, 429, 500, 502, 503, and 504 responses, with at most three requests, a 30-second limit per request, and two- and four-second delays. Permission, certificate, invalid-response, Azure login, and subscription errors fail without retry. Each job clears the Azure CLI account cache after cleanup. Run `make test-azure-oidc-login` to check this path locally without cloud access.
- **Radius control plane -> cloud.** The Radius pods (service accounts `ucp`, `applications-rp`, and `dynamic-rp` in `radius-system`) assume a cloud identity through the *cluster's own* OIDC issuer - Azure Workload Identity on Azure, IRSA on AWS - using a projected service-account token (audience `sts.amazonaws.com` for AWS). Radius reads the target identity from the credential registered with `rad credential register azure wi` and `rad credential register aws irsa`; enabling it requires installing the chart with `--set global.azureWorkloadIdentity.enabled=true` and `--set global.aws.irsa.enabled=true` (the LRT does this in [`.github/scripts/manage-radius-installation.sh`](../../../../.github/scripts/manage-radius-installation.sh)).

The identities are carried in repository secrets rather than in the workflow files:

| Secret                                             | Assumed by                                                          | Trust                                                     |
|----------------------------------------------------|---------------------------------------------------------------------|-----------------------------------------------------------|
| `AWS_GH_ACTIONS_ROLE`                              | the runner, via `configure-aws-credentials`                         | GitHub OIDC -> IAM role                                   |
| `FUNC_TEST_RAD_IRSA_ROLE`                          | the Radius pods, via IRSA                                           | cluster OIDC issuer -> IAM role                           |
| `AZURE_SP_TESTS_APPID` + `AZURE_SP_TESTS_TENANTID` | both the runner (Azure CLI) and the Radius pods (workload identity) | GitHub OIDC and AKS OIDC -> AAD app federated credentials |
| `FUNCTEST_AWS_ACCOUNT_ID`                          | the AWS account id in the environment `awsScope` (not a credential) | -                                                         |

#### Provision the AWS identities

The AWS identity resources are owned by the [`radius-project/wellknown`](https://github.com/radius-project/wellknown) Terraform layers. Do not create OIDC providers or edit role trust policies manually because a later Terraform apply will overwrite that drift.

Apply the layers in order whenever the LRT cluster is created or rotated:

1. `modules/20-functional-tests` publishes `workload_identity_issuers`, containing both the blob-hosted KinD issuer and the managed AKS issuer, plus the Radius service-account subjects.
2. [`modules/21-functional-tests-aws`](https://github.com/radius-project/wellknown/tree/main/modules/21-functional-tests-aws) creates or reuses an IAM OIDC provider for each issuer. It creates the IRSA role trusted by every issuer and the GitHub Actions role trusted by the Radius repository.
3. `modules/30-functional-tests-github` publishes the resulting role ARNs as `FUNC_TEST_RAD_IRSA_ROLE` and `AWS_GH_ACTIONS_ROLE`.

The GitHub Actions role allows 5400-second sessions, and the LRT workflow requests that duration to leave headroom over the 60-minute functional-test window. Its generated subject patterns include branch refs, so the scheduled `main` run uses `repo:radius-project/radius:ref:refs/heads/main` without a separate trust-policy edit.

### Terraform provider configuration test

`Test_TerraformRecipe_AzureResourceGroup` in `test/functional-portable/corerp/cloud/resources` checks both the registered Azure provider and a custom provider alias. The custom case loads the tenant from a secret store, loads the client ID through `envSecrets`, and enables OIDC through `env`. Invalid environment defaults ensure the explicit provider fields and secret-backed environment value must take effect. It uses the registered Azure workload identity and replaces the skipped PostgreSQL provider test; no database or fixed readiness delay is needed. The custom case uses a separate fixture, `corerp-resources-terraform-azurerg-custom-provider.bicep`, so neither fixture needs conditional resources.

Both cases keep their Kubernetes state Secret until normal Terraform destroy completes. The Terraform driver does not record Azure scope IDs, such as resource groups, as output resources, so the test uses the Azure SDK instead. It checks that the Azure resource group exists after deployment and that both the group and state Secret are absent after deletion, even when `RADIUS_TEST_FAST_CLEANUP=true`. SDK fallback cleanup runs only after those checks and cannot turn a failed destroy into a passing test.

Use the normal cloud-suite setup, including `AZURE_SUBSCRIPTION_ID`, the Terraform module server, and the same Entra service principal for the runner and Radius pods. The custom case reads `AZURE_SP_TESTS_TENANTID` and `AZURE_SP_TESTS_APPID`, or the service-principal identity from `az account show`. Publish both `azure-rg` and `azure-rg-custom-provider` with `make publish-test-terraform-recipes`, then run:

```bash
go test ./test/functional-portable/corerp/cloud/resources \
  -run '^Test_TerraformRecipe_AzureResourceGroup$' -count=20 -timeout 60m -v
```

### Terraform cloud backend lifecycle tests

`Test_TerraformCloudBackend_S3` and `Test_TerraformCloudBackend_AzureRM` live in `test/functional-portable/corerp/cloud/resources`. The existing `functional-test-cloud.yaml` → `corerp-cloud` matrix leg → `make test-functional-corerp-cloud` discovers both without build tags or an opt-in flag. Missing cloud credentials or configuration fail these tests rather than silently skipping them. They use the management API through `RPTest` to create `Radius.Core/terraformSettings`, a recipe pack, an environment and an application, then deploy real Terraform recipes through two resources.

All tests in this group deploy through the user-defined type `Test.CloudBackend/stateResource`, registered from `testdata/cloudbackend-resourcetypes.yaml` by `registerCloudBackendResourceType`, rather than `Applications.Core/extenders`, which is being deprecated. Registration is idempotent and resource types are shared, so it is not cleaned up. The type's schema is empty because the backend recipes declare no outputs.

Each backend test creates two independent states, updates only the first resource's `revision` tag, and destroys the resources separately. It reads the actual S3 objects or Azure blobs at independently calculated 40-hex state keys, verifies distinct lineages, stable lineage and increasing serial across the update, and checks the other state's bytes remain unchanged. SDK reads verify actual recipe-managed bucket/resource-group tags and disappearance before fallback cleanup runs. After each destroy, that resource's own state object must be gone, because Radius deletes the empty state Terraform leaves behind; this is what exercises the conditional delete against the real storage APIs. The other resource's state, the backing storage, and a decoy object seeded under the same prefix that Radius never owns must all survive untouched, and no lock or extra objects may remain. Both backends use a nested `e2e/<test name>` prefix: `keyPrefix` is required, so neither exercises a default. Full state contents and access tokens are never printed.

These tests always perform synchronous destruction and state-removal checks, even when CI sets `RADIUS_TEST_FAST_CLEANUP=true`. Unique, test-owned backend storage is deleted only afterward by bounded cleanup callbacks; cleanup failures fail the test. The S3 test creates an unversioned private state bucket and two empty recipe-managed buckets. The Azure test creates a Standard LRS storage account in the CI-owned resource group, a private container with shared-key authentication disabled, and two empty recipe-managed resource groups. Do not run against production accounts or subscriptions. Process termination can interrupt cleanup; use the test's unique `tfbackend-` names (`tfauth-` for the guard regressions below) and `radiustest=terraform-cloud-backend` cloud tags to investigate leftovers.

Azure setup has independent **10-minute ARM provisioning** and **10-minute Blob permission-propagation** budgets; provisioning does not consume the propagation allowance. The readiness probe writes and immediately deletes a temporary `radius-readiness-probe` blob rather than reading container metadata: blob writes are DataActions that the runner's subscription Contributor role does not grant, so a metadata read would succeed before the Storage Blob Data Contributor assignment had propagated and the suite's first real write would then fail. Only that probe retries expected authorization-propagation 403 responses, transient DNS/network failures, and SDK-retryable HTTP 408/429/500/502/503/504 responses. Permanent authentication, certificate and other HTTP failures stop immediately. Cancellation is preserved, and timeout diagnostics report the last error category/status without raw response bodies or tokens.

Normal Radius PUT/DELETE operations have a separate **15-minute** limit to allow cold Terraform binary/provider downloads; S3 SDK setup and individual cleanup callbacks remain bounded at **5 minutes**. A timed-out PUT can still be `Updating` server-side, so Radius cleanup retries 409 conflicts within its cleanup budget instead of force-deleting metadata under an active Terraform operation. Persistent conflicts and SDK fallback cleanup failures remain test failures. CI's existing **30-minute Go suite** and **35-minute test-step** ceilings are unchanged: stage limits are ceilings, not guaranteed time reservations, and multiple cold stages can exceed that shared suite budget. The dedicated command below allows 60 minutes for cold-start diagnosis; measure authorized live CI timings before deciding whether a separately budgeted cloud-backend lane is needed. Do not expand unrelated suites or treat timeout/cleanup failures as successful coverage.

`Test_TerraformCloudBackend_AzureRM_RejectsConflictingStateAuth` and `Test_TerraformCloudBackend_S3_RejectsStateEndpointOverride` are the native regressions for the execution-environment guard. Terraform resolves several environment variables ahead of the identity Radius renders into the generated backend block, so a stray one would silently authenticate state access as an unregistered credential. Each test puts one such variable in the TerraformSettings `env` map and asserts the deployment is refused, naming the variable.

Creating the settings must still succeed: those variables are legitimate for recipe providers and are rejected only for executions that use a cloud backend. Because Radius rejects before Terraform is invoked, these tests need no provisioned storage and name storage that does not exist. That is what makes the assertion sharp rather than merely checking that something failed — a working guard fails naming the variable, while a regressed one would reach `terraform init` and fail with a storage error, which the tests assert against explicitly.

The same guard runs on the delete path, so a resource rejected at deploy time cannot be deleted until the offending variable is removed from its TerraformSettings. The tests assert that rejection instead of working around it, which doubles as delete-path regression coverage, and therefore leave the resource record behind on purpose. Nothing was provisioned, so the leftover is a Radius record that leaks no cloud resources; the application, environment, recipe pack and settings are still deleted. Operators who hit this outside tests recover by removing the variable from the settings and retrying the delete.

**Authentication coverage:** CI tests the registered AWS **IRSA** and Azure **Workload Identity** credentials only. The runner uses its existing AWS assumed-role session. For its own Azure calls the test builds a credential rather than relying on the `azure/login` session directly: when the GitHub OIDC inputs (`ACTIONS_ID_TOKEN_REQUEST_URL` and `ACTIONS_ID_TOKEN_REQUEST_TOKEN`) and the service principal identity are both available, it uses `ClientAssertionCredential` and mints a fresh federated assertion on every token acquisition. The identity comes from `AZURE_SP_TESTS_TENANTID`/`AZURE_SP_TESTS_APPID`, falling back to the `az account show` record because this suite runs under `pull_request_target`, where workflow env additions are not visible until they merge. `DefaultAzureCredential` is the fallback used only when those inputs are missing, which is the usual case outside CI. Minting per acquisition is what avoids `AADSTS700024`: the assertion cached by `azure/login` expires within minutes, so a scope not already cached at login — the Blob data plane, for example — otherwise fails later in the job even though the ARM token is still valid. Check this path first when Azure authentication fails. It does not register or replace credentials. AccessKey and ServicePrincipal backend authentication, cross-cloud recipe/backend combinations, migration, and backend immutability are not exercised by these E2E tests. Neither is the precondition-failure branch of the conditional state delete: that branch needs a competing write to land between the read and the delete, which cannot be induced from outside the control plane. It is covered in `pkg/recipes/terraform/backend_cleanup_test.go`, which drives the real S3 and Azure Blob SDKs against a stub that evaluates `If-Match` itself and rewrites the object between the read and the delete.

Prerequisites, in addition to the normal cloud-suite setup:

- Install the Radius build containing cloud backends and publish the checked-out `backend-s3` and `backend-azurerm` recipes with `make publish-test-terraform-recipes`. The existing publisher automatically includes these directories. The cluster must reach the Terraform provider registry.
- Set `TF_RECIPE_MODULE_SERVER_URL`. For S3, set `AWS_REGION` and `AWS_ACCOUNT_ID`, with runner credentials for that account. Runner and registered IRSA roles need S3 bucket creation/deletion, tagging, listing and object read/write/delete permissions; the existing `wellknown/modules/21-functional-tests-aws` test policy provides these. Endpoint overrides are rejected, not redirected to emulators.
- For Azure, set `AZURE_SUBSCRIPTION_ID`, `AZURE_LOCATION` and `INTEGRATION_TEST_RESOURCE_GROUP_NAME`. The runner and registered Workload Identity must use the **same Entra service principal**, as in cloud CI. The runner needs storage-account and resource-group management plus role-assignment create/delete permissions. The existing `wellknown/modules/20-functional-tests` setup grants Contributor and Role Based Access Control Administrator. The test temporarily assigns **Storage Blob Data Contributor only on its new container** to this principal, waits for data-plane access, and removes that assignment during cleanup. No account keys, new CI secrets, broad role grants or preprovisioned state storage are required. A runner logged in as a personal Azure user is not a supported substitute.

After configuring a dedicated cloud test cluster, run the live checks:

```bash
make publish-test-terraform-recipes
go test ./test/functional-portable/corerp/cloud/resources \
  -run '^Test_TerraformCloudBackend_' -count=1 -timeout 60m -v
```

For local validation **without cloud operations**, compile the E2E package and run only its pure helpers (there is no `TestMain` in this package):

```bash
mkdir -p dist/test
go test -c -o dist/test/cloud-resources.test ./test/functional-portable/corerp/cloud/resources
go test ./test/functional-portable/corerp/cloud/resources -run '^TestTerraformCloud' -count=1
go test ./pkg/recipes/terraform/config/backends ./pkg/corerp/datamodel
terraform fmt -check test/testrecipes/test-terraform-recipes/backend-s3 test/testrecipes/test-terraform-recipes/backend-azurerm
```

If setup fails with Azure authorization errors, verify the documented shared principal and CI role assignments rather than bypassing the failure with a skip or enabling account keys. If a recipe changes, republish its zip before rerunning. Local compilation and helper tests do not demonstrate working cloud authentication; only the live checks do.

### Existing-environment troubleshooting

- **You changed a recipe.** Republish the Bicep or Terraform test recipes so the cluster uses the updated content.
- **A Terraform recipe test cannot find its module.** Run `make publish-test-terraform-recipes` against the test cluster and set `TF_RECIPE_MODULE_SERVER_URL` to the module-server URL reachable from the deployment.
- **Tests cannot pull a package.** Confirm packages published to your organization are publicly readable.
- **Bicep reports that a local registry is not trusted.** Set `BICEP_TRUSTED_REGISTRIES` as described in [Local Bicep registry trust](#local-bicep-registry-trust). Fix certificate errors in the local CA setup rather than disabling verification.
- **You changed the `rad` CLI.** Put the rebuilt binary on `PATH`, or set `RAD_PATH` for CodeLens.
- **Many tests fail immediately.** Confirm the active Kubernetes namespace is `default`.
- **A special test group fails during setup.** Confirm the cluster meets the requirements in [Run a special test group](#run-a-special-test-group).
- **Long-running AWS tests fail with `AccessDenied` on `AssumeRoleWithWebIdentity`.** Confirm the three `radius-project/wellknown` functional-test layers were applied in order and that the current `FUNC_TEST_RAD_IRSA_ROLE` secret matches the generated role ARN.
