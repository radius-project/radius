---
type: docs
title: "Running Radius end-to-end tests"
linkTitle: "Radius End-To-End Tests"
description: "How to run Radius end-to-end tests"
weight: 100
---


These tests create a Radius environment, deploy a bicep template and verify the deployment


## Environment variables

These tests rely on the following environment variables:-

```
export PATH=$PATH:<Radius Binary Path>
export AZURE_TENANT_ID=Tenant ID of the Azure subscription
export AZURE_CLIENT_ID=App ID of the Service Principal
export AZURE_CLIENT_SECRET=Password for the Service Principal
export INTEGRATION_TEST_SUBSCRIPTION_ID=Azure subscription ID
export INTEGRATION_TEST_DEFAULT_LOCATION=Azure location to be used
export INTEGRATION_TEST_BASE_GROUP_NAME=Prefix of the resource group created. This will be appended by a random suffix to ensure uniqueness
```


## Running the tests via GitHub workflow

These tests automatically run nightly on the main branch using the e2e-tests.yaml github workflow. They can be run on-demand for a PR by adding a comment "/run-e2e-tests".


## Running the tests locally

1. Create a [service principal](https://docs.microsoft.com/en-us/azure/active-directory/develop/howto-create-service-principal-portal), specify a password and assign it "Owner" role.
2. Export the environment variables mentioned above
3. Run:
    ```
    make e2e-tests
    ```
