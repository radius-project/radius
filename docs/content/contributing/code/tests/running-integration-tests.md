---
type: docs
title: "Running Radius integration tests"
linkTitle: "Deploy tests"
description: "How to run Radius integration tests"
weight: 200
---

These tests verify whether:

- That Radius environments can be created on Azure successfully.
- That Bicep templates of sample applications ca be deployed to the Radius environment. 

These run on Azure Radius environments (also called Radius test clusters) that are managed dynamically as part of the test process.

## Running via GitHub workflow

These tests automatically run for every PR in the `build.yaml` github workflow.

We do not run these tests for commits to `main` or tags since they might block the build if they fail.

We use a CLI tool (`test-env`) to manage operations to make this work:

- `test-env reserve ...` to wait for a cluster to be ready and reserve it
- `test-env update-rp ...` to update the control plane for testing
- `test-env release ...` to release the lease 
- `test-env register ...` to register an environment
- `test-env unregister ...` to delete an environment

### How this works 

For each PR we run the following set of steps:

- Trigger the creation of a new test environment (`create-environment.yaml) and allow it to complete asynchronously.
- Reserve an available environment from the list of environments.
- Run deployment tests.
- Trigger the asynchronous deletion of the specific test environment we used .

Some notes about how/why we do this:

- We want to ensure we're testing environment setup regularly but don't want to make PRs wait synchronously. If one of these async workflows fails, it will open an issue.
- We randomize the Azure region used for environment creation, this allows good coverage of the regions we support.
- We delete test environments after each run to make sure that a buggy PR doesn't pollute our pool of environments.

## Configuration

These tests use your local Azure credentials, and Radius environment for testing. In a GitHub workflow, our automation makes the CI environment resemble a local dev scenario.

The tests use our product functionality (the Radius config file) to configure the environment.

## Running the tests locally

1. Create an environment (`rad env init azure -i`)
2. Merge your AKS credentials to your kubeconfig (`rad env merge-credentials --name azure`)
3. Place `rad` on your path
4. Run:

    ```sh
    make integration-tests
    ```

When you're running locally with this configuration, the tests will use your locally selected Radius environment and your local copy of `rad`.

## Controlling test environments

You can use a set of commands inside the repository to automate test environment maintainance. These commands are triggered by **comments on a Pull Request**. You can see the commands defined in `radius-bot.yaml`.

Useful commands:

- `/create-environment`
- `/delete-environment <environment name>`

The `/create-environment` command always uses the `main` branch to create environments.

### Example: PR is stuck

If you have a PR stuck on the *reserving test environment step* it means that there are no available test environments.

You can imperatively create a new one by posting a comment on your PR:

```txt
/create-environment
```

### Example: Updating environment setup

If you have merged changes to `main` that impact environment setup, you can trigger a series of operations that will cycle the environments by using the create and delete commands (one per comment).
