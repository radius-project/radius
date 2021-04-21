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

We use a CLI tool (`test-env`) to manage operations to make this work:

- `test-env reserve ...` to wait for a cluster to be ready and reserve it
- `test-env update-rp ...` to update the control plane for testing
- `test-env release ...` to release the lease 

### Configuration

These tests use your local Azure credentials, and Radius environment for testing. In a GitHub workflow, our automation makes the CI environment resemble a local dev scenario.

## Running the tests locally

1. Create an environment (`rad env init azure -i`)
2. Merge your AKS credentials to your kubeconfig (`rad env merge-credentials --name azure`)
3. Place `rad` on your path
4. Run:

    ```sh
    make deploy-tests
    ```

When you're running locally with this configuration, the tests will use your locally selected Radius environment and your local copy of `rad`.


## Adding new test clusters

1. Create a radius Azure environment using:-
    ```
    rad env init azure -i
    ```
2. Add an entry to the `envionments` table of the `deploytests` storage account
3. Add tags to the resource group: RadiusTests:DO-NOT-DELETE using the Azure portal
