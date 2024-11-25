# Running Radius functional tests

You can find the functional tests under `./test/functional`. A functional test (in our terminology) is a test that interacts with real hosting environments (Kubernetes), deploys real applications and resources, and covers realistic or simulated user scenarios.

These tests verify whether:

- That Radius Environment can be created successfully.
- That Bicep templates of sample applications can be deployed to the Radius Environment.

## Running via GitHub workflow

These tests automatically run for every PR in the `functional-tests.yml` github workflow.

We do not run these tests for commits to `main` or tags since they might block the build if they fail.

### How this works

For each PR we run the following set of steps:

- Build Radius and publish test assets
- For each group of tests:
  - Create a Kubernetes cluster and install the build
  - Run tests
  - Delete any cloud resources that were created

We have a separate scheduled job (`purge-test-resources.yaml`) that will delete cloud resources that are left behind. This can happen when the test run is cancelled or times out.

## Configuration

These tests use your local Kubernetes credentials, and Radius Environment for testing. In a GitHub workflow, our automation makes the CI environment resemble a real user scenario. This way we test a setup that is close to what users will have in the real world.

As much as possible, the tests use product functionality such as the Radius CLI configuration and local KubeConfig to detect settings.

## Running the tests locally

**Please note** that tests may be cached if you are running them a few times in a row. Please run `go clean -testcache` to make sure that the cache is invalidated before running a test.

### Prerequisites

1. Place `rad` on your path
2. Make sure `rad-bicep` is downloaded (`rad bicep download`)
3. Make sure your [local dev environment is setup](../contributing-code-control-plane/running-controlplane-locally.md)
4. Log into your Github account and [Generate PAT](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/managing-your-personal-access-tokens)
5. Log-in to the container registry of your Github organization.
   1. `export CR_PAT=<your_pat>`
   2. `echo $CR_PAT | docker login ghcr.io -u <your_username> --password-stdin`
6. Publish Bicep test recipes by running `BICEP_RECIPE_REGISTRY=<registry-name> make publish-test-bicep-recipes`
7. Publish Terraform test recipes by running `make publish-test-terraform-recipes`
8. Change the visibility of the published packages to 'public'

> ⚠️ The tests assume the Kubernetes namespace in use is `default`. If your environment is set up differently you will see test failures.
> ⚠️ If you set environment variables for functional tests you may need to restart VS Code or other editors for them to take effect.

### Run Non-Cloud Functional Tests

1. Required environment variables:
   1. **TF_RECIPE_MODULE_SERVER_URL**: This is the URL for the Terraform Recipe Module Server. If you have run `make publish-test-terraform-recipes` you will see this URL at the end of that command.
   2. **DOCKER_REGISTRY**: This is the container registry that you would be using for storing the test related images.
   3. **BICEP_RECIPE_REGISTRY**: This is the container registry that you would be using for storing the Bicep recipes.
   4. **RADIUS_SAMPLES_REPO_ROOT**: This should point to the root directory of the Samples repository in your local, if you want to run the functional tests for the samples.
2. Run:

   ```sh
       .{workspace}/radius/test/execute-functional-tests-noncloud.sh
   ```

When you're running locally with this configuration, the script is going to create a new Radius group and environment that the functional tests need to run. The same script is also going to make sure that the necessary environment variables are set. If everything is set, the script will run the commands:

```sh
    make test-functional-cli-noncloud
    make test-functional-corerp-noncloud
    make test-functional-daprrp-noncloud
    make test-functional-datastoresrp-noncloud
    make test-functional-kubernetes-noncloud
    make test-functional-msgrp-noncloud
    make test-functional-samples-noncloud
    make test-functional-ucp-noncloud
```

### Run Cloud Functional Tests

1. Required environment variables:
   1. **TF_RECIPE_MODULE_SERVER_URL**
   2. **DOCKER_REGISTRY**
   3. **BICEP_RECIPE_REGISTRY**
   4. **AZURE_COSMOS_MONGODB_ACCOUNT_ID**
   5. **AWS_ACCESS_KEY_ID**
   6. **AWS_SECRET_ACCESS_KEY**
   7. **AWS_REGION**
2. You also need to create AWS and Azure Credentials. Please refer to: <https://docs.radapp.io/reference/cli/rad_credential_register/>.
3. Run:

   ```sh
       .{workspace}/radius/test/execute-functional-tests-cloud.sh
   ```

When you're running locally with this configuration, the script is going to create a new Radius group and environment that the functional tests need to run. The same script is also going to make sure that the necessary environment variables are set. If everything is set, the script will run the commands:

```sh
    make test-functional-corerp-cloud
    make test-functional-ucp-cloud
```

You can also run/debug individual tests from VSCode.

### Tips

> 💡 If you make changes to recipes, make sure to re-run the _publish test recipe_ step from prerequisites.
> 💡 Make sure the packages published to your organization have their visibility set to "public"
> 💡 If you make changes to the `rad` CLI make sure to copy it to your path.

### Seeing log output

Some of these tests take a few minutes to run since they interact with cloud resources. You should configure VSCode to output verbose output so you can see the progress.

Open your VSCode `settings.json` with the command `Preferences: Open Settings (JSON)` and configure the following options:

```json
{
    ...
    "go.testTimeout": "60m",
    "go.testFlags": [
        "-v"
    ],
}
```

### Using Codelens (VSCode)

VSCode will start a child process when you execute a `'run test'/'debug test'` codelens action (see image for example). If you are using this to run functional tests, this process may not resolve `rad` correctly. You can specify environment variables for codelens using `settings.json`:

```json
{
    ...
    "go.testEnvVars": {
        "RAD_PATH": "${workspaceFolder}/dist/linux_amd64/release"
    },
}
```

![Screenshot of VS Code Codelens UI](./vscode_debug_test.png)
