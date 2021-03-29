---
type: docs
title: "Running Radius deploy tests"
linkTitle: "Deploy tests"
description: "How to run Radius deploy tests"
weight: 200
---


These tests verify whether the bicep template deployment to the Radius environment succeeded. These run on pre-created radius environments (or Radius test clusters).


## Environment variables

These tests rely on the following environment variables:-

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

## Running via GitHub workflow

These tests automatically run for every PR in the `build.yaml` github workflow.


## Running the tests locally

1. Create a [service principal](https://docs.microsoft.com/en-us/azure/active-directory/develop/howto-create-service-principal-portal), specify a password and assign it "Owner" role.
2. Export the environment variables mentioned above
3. Run:
    ```
    make deploy-tests
    ```

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
