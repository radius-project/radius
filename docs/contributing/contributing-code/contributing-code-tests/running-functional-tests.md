# Run functional tests locally

## Purpose

Use this guide to build Radius from your checkout and run the non-cloud functional tests in local k3d clusters. The test runner creates its own Kubernetes clusters, Radius configuration, container registry, and test fixtures. It does not use your current Kubernetes context or Radius workspace.

You do not need a cloud account, GitHub personal access token, or external container registry.

## Prerequisites

Use Debian/Ubuntu Linux with an `amd64` processor, or macOS with an `amd64` or `arm64` processor. Linux certificate setup uses `update-ca-certificates` and `/usr/local/share/ca-certificates`; other Linux distributions are not supported by this runner. Linux `arm64` is not supported because the current image-build targets produce `linux/amd64` images.

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

Start Docker before running the tests. The test runner installs the repository-pinned Bicep and Dapr CLIs.

The test runner also requires:

- `sudo` access to trust a temporary registry certificate and add `radius-local-registry` to `/etc/hosts`.
- Free TCP ports 5443, 6550-6555, and 30080.
- On Linux, at least 1024 inotify instances per user. Check the current value with `sysctl fs.inotify.max_user_instances`.

### macOS tool selection

Use Docker Desktop with Linux containers. On Apple Silicon, enable support for running `linux/amd64` containers: the current `make docker-build` target builds the Radius server images for that platform, while the runner builds `rad` for your Mac's native architecture.

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

1. Builds the `rad` CLI and Radius container images from your checkout.
2. Creates a local container registry and a k3d cluster.
3. Installs Radius and the test fixtures.
4. Runs the standard non-cloud functional tests.
5. Runs the database, multicluster, statestore, and upgrade test groups sequentially in separate clusters.

Each setup refreshes the retained samples checkout from its `edge` branch and archives previous test reports and container logs before starting a new run. Test groups always run sequentially, even when your Make settings request parallel execution.

Do not start another local functional test run at the same time. The runs share temporary Bicep configuration files.

### 3. Clean up

After reviewing the results, remove the clusters, registry, trusted certificate, and `/etc/hosts` entry:

```bash
bash build/scripts/local-functional.sh cleanup
```

The command retains logs and test configuration under `dist/local-functional/`.

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

Replace `database` with `multicluster`, `statestore`, or `upgrade` as needed. Do not run an isolated group concurrently with a standard test group.

## Verification

The complete test run succeeds when `make test-functional-local-noncloud` exits with status 0.

Test output and per-group gotestsum JSON files are stored under `dist/local-functional/results/`. Container logs are stored under `dist/local-functional/container_logs/`.

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

Run `make test-local-functional-runner` to check the runner's platform preflight, fixture locking/restoration, diagnostics rotation, serial execution, and cleanup using mocked infrastructure commands. These regression checks also run as part of `make test` and do not require Docker, Kubernetes, or sudo access.

## Troubleshooting

### Existing test infrastructure

Setup stops instead of replacing an existing test cluster or registry. Remove the previous test infrastructure, then start the test run again:

```bash
bash build/scripts/local-functional.sh cleanup
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

Replace `database` with `multicluster`, `statestore`, or `upgrade`.

Retry the matching test target against the retained cluster:

```bash
LOCAL_FUNCTIONAL_SUITE=database \
  bash build/scripts/local-functional.sh test test-functional-database-noncloud
```

### A complete test run failed

Review `dist/local-functional/results/` and `dist/local-functional/container_logs/`. Run cleanup before starting another complete test run because failed tests can leave resources and provider registrations behind.

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

The supported groups and variants are defined in [`build/test.mk`](../../../../build/test.mk).

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

### Cloud credentials in CI

Cloud workflows use federated identity instead of static credentials. GitHub Actions exchanges GitHub OIDC tokens through the cloud login actions, while Radius pods use Azure Workload Identity or AWS IAM Roles for Service Accounts.

The AWS identities are managed by the `radius-project/wellknown` Terraform layers. Apply `modules/20-functional-tests`, `modules/21-functional-tests-aws`, and `modules/30-functional-tests-github` in order when the long-running test cluster is created or rotated.

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
