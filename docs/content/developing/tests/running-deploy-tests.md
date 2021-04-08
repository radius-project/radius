---
type: docs
title: "Running Radius deploy tests"
linkTitle: "Deploy tests"
description: "How to run Radius deploy tests"
weight: 200
---

These tests verify whether the bicep template deployment to the Radius environment succeeded. These run on pre-created radius environments (or Radius test clusters).

## Running via GitHub workflow

These tests automatically run for every PR in the `build.yaml` github workflow.

### Configuration

These tests rely on the following environment variables for configuration:

```
export PATH=$PATH:<Radius Binary Path>
export AZURE_TENANT_ID=Tenant ID of the Azure subscription
export AZURE_CLIENT_ID=App ID of the Service Principal
export AZURE_CLIENT_SECRET=Password for the Service Principal
export INTEGRATION_TEST_SUBSCRIPTION_ID=Azure subscription ID
export RP_DEPLOY=true
export RP_IMAGE=docker image for the RP
```

`RP_DEPLOY` and `RP_IMAGE` are optional. If you set `RP_DEPLOY=true`, then the tests will deploy the image specified by `RP_IMAGE` to the test environment. You do not need to worry about cleanup, because every deploy tests job will deploy its own copy of the image.

## Running the tests locally

1. Create an environment (`rad env init azure -i`)
2. Merge your AKS credentials to your kubeconfig (`rad env merge-credentials --name azure`)
3. Place `rad` on your path
4. Run:

    ```sh
    make deploy-tests
    ```

When you're running locally with this configuration, the tests will use your locally selected Radius environment and your local copy of `rad`.

You do not need to configure any environment variables to run the tests from your machine. You may want to configure `RP_DEPLOY` and `RP_IMAGE` to deploy a private build of your RP.

## Adding new test clusters

1. Create a radius Azure environment using:-
    ```
    rad env init azure -i
    ```
2. Copy the radius config created to the test configuration:-
    ```
    cp ($HOME/.rad/config.yaml) to test/deploy-tests/<resource group of the env>.yaml
    ```
3. Add the resource group of the radius environment created to the list of available test clusters in test/deploy-tests/deploy-tests-clusters.txt
4. Add tags to the resource group: RadiusTests:DO-NOT-DELETE using the Azure portal
