# Running Radius functional tests

## Purpose

Functional tests (also called end-to-end tests) interact with real hosting environments (Kubernetes), deploy real applications and resources, and cover realistic user scenarios. They verify, for example, that a Radius Environment can be created successfully and that the Bicep templates of sample applications can be deployed to it. This page is for contributors validating a change against a real cluster; for the full set of test tiers and when to run each, start at the [test matrix overview](./README.md).

The tests live under `./test/functional-portable`. They use product functionality - the Radius CLI configuration and your local KubeConfig - to detect settings, so the local setup resembles a real user scenario.

## Prerequisites

1. Place `rad` on your path.
2. Make sure `bicep` is downloaded (`rad bicep download`).
3. Make sure your [local dev environment is set up](../contributing-code-debugging/radius-os-processes-debugging.md).
4. Log into your GitHub account and [generate a PAT](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens).
5. Log in to the container registry of your GitHub organization:

   ```sh
   export CR_PAT=<your_pat>
   echo $CR_PAT | docker login ghcr.io -u <your_username> --password-stdin
   ```

6. Publish the Bicep test recipes: `BICEP_RECIPE_REGISTRY=<registry-name> make publish-test-bicep-recipes`.
7. Publish the Terraform test recipes: `make publish-test-terraform-recipes`.
8. Change the visibility of the published packages to `public`.

> ⚠️ The tests assume the Kubernetes namespace in use is `default`. If your environment is set up differently you will see test failures.
>
> ⚠️ If you set environment variables for functional tests you may need to restart VS Code or other editors for them to take effect.

## Steps

### Run the tests locally

Run:

```sh
./test/executeFunctionalTest.sh <resourcegroup_name>
```

When you run locally with this configuration, the tests use your locally selected Radius Environment and your local copy of `rad`. `executeFunctionalTest.sh` creates the Azure resources, exports the values used by the tests, and runs:

```sh
make test-functional-corerp
make test-functional-msgrp
make test-functional-daprrp
make test-functional-datastoresrp
```

To run a single group directly, call its `make` target — for example `make test-functional-corerp-noncloud` for the non-cloud Core RP tests, or `make test-functional-all-noncloud` for the standard non-cloud groups. The groups (`ucp`, `kubernetes`, `corerp`, `cli`, `msgrp`, `daprrp`, `datastoresrp`, `dynamicrp`, `samples`, `upgrade`, `multicluster`, and `statestore`) and the variants each group supports are defined in [`build/test.mk`](../../../../build/test.mk).

You can also run or debug individual tests from VS Code.

### Run a special test group

The aggregate `make test-functional-all-noncloud` target intentionally excludes these isolated groups:

| Target                                       | Requirements and behavior                                                                                                                      |
|----------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| `make test-functional-multicluster-noncloud` | Requires a second Kubernetes cluster, a target-cluster Secret mounted into Radius, and `RADIUS_TEST_EXTERNAL_KUBECONFIG` for the test process. |
| `make test-functional-statestore-noncloud`   | Destructive lifecycle test that installs, purges, and reinstalls Radius. Run it only on a dedicated cluster.                                   |
| `make test-functional-upgrade-noncloud`      | Exercises the Radius upgrade path and performs its own install/upgrade lifecycle.                                                              |

The multicluster and statestore groups run as isolated CI legs in `functional-test-noncloud.yaml`; do not run them against a shared development cluster.

For multicluster tests, create the namespace and Secret before installing Radius. The kubeconfig stored in the Secret must use an API-server address reachable from the Radius pods:

```bash
kubectl create namespace radius-system --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic target-kubeconfig \
  --namespace radius-system \
  --from-file=kubeconfig=<pod-reachable-external-kubeconfig>
```

Install Radius with `global.targetCluster.enabled=true`, then set `RADIUS_TEST_EXTERNAL_KUBECONFIG` to the host-side kubeconfig that the test process uses to assert where resources were created.

### Configure test execution

The Make targets accept these environment variables:

| Variable                          | Purpose                                                                |
|-----------------------------------|------------------------------------------------------------------------|
| `TEST_TIMEOUT`                    | Overrides the Go test timeout. The default in `build/test.mk` is `1h`. |
| `RADIUS_TEST_EXTERNAL_KUBECONFIG` | Points multicluster tests at the external workload cluster.            |
| `TF_RECIPE_MODULE_SERVER_URL`     | Overrides the Terraform recipe module server URL.                      |
| `RADIUS_TEST_FAST_CLEANUP`        | Selects standard or fast cleanup as described below.                   |

### Control test cleanup

Functional tests support two cleanup modes, selected with the `RADIUS_TEST_FAST_CLEANUP` environment variable:

- **Standard cleanup** (default for local development): waits for each resource to be fully deleted before proceeding, logs the deletion process, and shows retries for resources stuck in "Updating". Best for debugging cleanup issues.
- **Fast cleanup** (default for CI): initiates deletions in the background without waiting, which avoids deletion timeouts and dramatically reduces run time. It **skips post-delete verification**, so it is only safe for non-cloud tests where Kubernetes cluster cleanup handles orphaned resources. CI enables it with `RADIUS_TEST_FAST_CLEANUP=true`.

```bash
# Enable fast cleanup (useful for local testing with unique resource names)
export RADIUS_TEST_FAST_CLEANUP=true
go test ./test/functional-portable/corerp/noncloud/resources

# Disable fast cleanup for debugging (default for local development)
export RADIUS_TEST_FAST_CLEANUP=false
go test ./test/functional-portable/corerp/noncloud/resources
```

> ⚠️ **Important**: Fast cleanup is only safe for non-cloud tests. Cloud tests always use standard cleanup to ensure proper deletion of cloud resources that incur costs.

### See log output in VS Code

Some tests take a few minutes because they interact with cloud resources. Configure VS Code to show verbose output so you can follow progress. Open `settings.json` with **Preferences: Open Settings (JSON)** and set:

```json
{
    "go.testTimeout": "60m",
    "go.testFlags": [
        "-v"
    ]
}
```

### Use Codelens (VS Code)

VS Code starts a child process when you use a `run test`/`debug test` Codelens action. That process may not resolve `rad` correctly. Specify environment variables for Codelens in `settings.json`:

```json
{
    "go.testEnvVars": {
        "RAD_PATH": "${workspaceFolder}/dist/linux_amd64/release"
    }
}
```

![Screenshot of VS Code Codelens UI](./vscode_debug_test.png)

### How the tests run in CI

These tests run automatically for every PR via the `functional-test-noncloud.yaml` and `functional-test-cloud.yaml` GitHub workflows. We do not run them for commits to `main` or for tags, since a failure could block the build. For each PR, CI:

- Builds Radius and publishes the test assets.
- For each group of tests: creates a Kubernetes cluster, installs the build, runs the tests, and deletes any cloud resources that were created.

Separate scheduled jobs (`purge-azure-test-resources.yaml` and `purge-aws-test-resources.yaml`) delete cloud resources left behind when a run is cancelled or times out.

### Cloud credentials in CI (federated identity)

The cloud CI workflows - `functional-test-cloud.yaml`, the long-running test `long-running-azure.yaml` ("LRT"), and the two scheduled purge jobs - authenticate to Azure and AWS with **federated identity only**. No static cloud secrets (service-principal passwords or AWS access keys) are stored in GitHub; every credential is a short-lived token minted from an OIDC trust. Two distinct trusts are in play:

- **Runner -> cloud.** The GitHub Actions runner gets tokens from GitHub's OIDC provider (`token.actions.githubusercontent.com`), and `azure/login` / `aws-actions/configure-aws-credentials` exchange them for short-lived credentials that the test and purge code use to verify and delete cloud resources. Every such job sets `permissions: id-token: write`.
- **Radius control plane -> cloud.** The Radius pods (service accounts `ucp`, `applications-rp`, and `dynamic-rp` in `radius-system`) assume a cloud identity through the *cluster's own* OIDC issuer - Azure Workload Identity on Azure, IRSA on AWS - using a projected service-account token (audience `sts.amazonaws.com` for AWS). Radius reads the target identity from the credential registered with `rad credential register azure wi` and `rad credential register aws irsa`; enabling it requires installing the chart with `--set global.azureWorkloadIdentity.enabled=true` and `--set global.aws.irsa.enabled=true` (the LRT does this in [`.github/scripts/manage-radius-installation.sh`](../../../../.github/scripts/manage-radius-installation.sh)).

The identities are carried in repository secrets rather than in the workflow files:

| Secret                                             | Assumed by                                                              | Trust                                                     |
|----------------------------------------------------|-------------------------------------------------------------------------|-----------------------------------------------------------|
| `AWS_GH_ACTIONS_ROLE`                              | the runner, via `configure-aws-credentials`                             | GitHub OIDC -> IAM role                                   |
| `FUNC_TEST_RAD_IRSA_ROLE`                          | the Radius pods, via IRSA                                               | cluster OIDC issuer -> IAM role                           |
| `AZURE_SP_TESTS_APPID` + `AZURE_SP_TESTS_TENANTID` | both the runner (`azure/login`) and the Radius pods (workload identity) | GitHub OIDC and AKS OIDC -> AAD app federated credentials |
| `FUNCTEST_AWS_ACCOUNT_ID`                          | the AWS account id passed to `rad env update` (not a credential)        | -                                                         |

#### Point the AWS IRSA role at the LRT cluster

`FUNC_TEST_RAD_IRSA_ROLE` is shared with `functional-test-cloud.yaml`, which runs on a KinD cluster whose OIDC issuer is an Azure-blob issuer. The LRT runs on a pre-provisioned AKS cluster with a *different* OIDC issuer, so the role's trust policy must also trust that issuer. Register the AKS issuer as an IAM OIDC provider and append a trust statement for the three Radius service accounts. Run this once, when the LRT cluster is created or rotated:

```bash
export AWS_ACCOUNT_ID="<test-account-id>"            # = FUNCTEST_AWS_ACCOUNT_ID
export AKS_CLUSTER_NAME="<LRT_AKS_CLUSTER_NAME>"     # = vars.LRT_AKS_CLUSTER_NAME
export AKS_RESOURCE_GROUP="<LRT_AKS_RESOURCE_GROUP>" # = vars.LRT_AKS_RESOURCE_GROUP
IRSA_ROLE_NAME="$(basename "<FUNC_TEST_RAD_IRSA_ROLE-arn>")"

# 1. Fetch the AKS OIDC issuer and register it as an IAM OIDC provider (skip if it already exists).
AKS_OIDC_ISSUER="$(az aks show -n "$AKS_CLUSTER_NAME" -g "$AKS_RESOURCE_GROUP" --query 'oidcIssuerProfile.issuerUrl' -o tsv)"
ISSUER_HOST="$(echo "$AKS_OIDC_ISSUER" | sed -e 's~^https://~~' -e 's~/.*$~~')"
THUMBPRINT="$(echo | openssl s_client -servername "$ISSUER_HOST" -connect "${ISSUER_HOST}:443" \
  | openssl x509 -fingerprint -sha1 -noout | cut -d= -f2 | tr -d ':' | tr 'A-F' 'a-f')"
aws iam create-open-id-connect-provider --url "$AKS_OIDC_ISSUER" \
  --client-id-list "sts.amazonaws.com" --thumbprint-list "$THUMBPRINT"

# 2. Append a trust statement for the Radius service accounts on the AKS issuer.
OIDC_PROVIDER="${AKS_OIDC_ISSUER#https://}"
aws iam get-role --role-name "$IRSA_ROLE_NAME" --query 'Role.AssumeRolePolicyDocument' > trust.json
cat > aks-statement.json <<EOF
{
  "Effect": "Allow",
  "Principal": { "Federated": "arn:aws:iam::${AWS_ACCOUNT_ID}:oidc-provider/${OIDC_PROVIDER}" },
  "Action": "sts:AssumeRoleWithWebIdentity",
  "Condition": { "StringEquals": {
    "${OIDC_PROVIDER}:aud": "sts.amazonaws.com",
    "${OIDC_PROVIDER}:sub": [
      "system:serviceaccount:radius-system:ucp",
      "system:serviceaccount:radius-system:applications-rp",
      "system:serviceaccount:radius-system:dynamic-rp"
    ]
  } }
}
EOF
jq '.Statement += [input]' trust.json aks-statement.json > trust-updated.json
aws iam update-assume-role-policy --role-name "$IRSA_ROLE_NAME" --policy-document file://trust-updated.json
```

`AWS_GH_ACTIONS_ROLE` needs no cluster-specific change, but because the LRT is triggered on a schedule its OIDC subject is `repo:radius-project/radius:ref:refs/heads/main`; confirm the role's trust `sub` condition matches (widen it if it was scoped only to pull requests) and that its `MaxSessionDuration` is at least the 60-minute functional-test window.

## Verification

- Each group prints `ok` (or the `gotestsum` summary) per package and `go test` exits non-zero on any failure.
- A successful run creates a Radius Environment, deploys the sample applications, asserts on their state, and then cleans up the resources it created.

## Troubleshooting

- **You changed a recipe.** Re-run the *publish test recipe* prerequisite step so the cluster uses your updated recipe.
- **Tests cannot pull a package.** Confirm the packages published to your organization have their visibility set to `public`.
- **You changed the `rad` CLI.** Copy the rebuilt `rad` to your path (or set `RAD_PATH` for Codelens) so the tests use your new binary.
- **Environment variables seem ignored.** Restart VS Code or your editor so newly set variables take effect.
- **Many tests fail immediately.** Confirm the Kubernetes namespace in use is `default`.
- **A special test group is skipped or fails during setup.** Confirm that you met the isolated-cluster requirements in [Run a special test group](#run-a-special-test-group).
