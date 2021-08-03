---
type: docs
title: "Running Radius Kubernetes functional tests"
linkTitle: "Functional tests"
description: "How to run Radius Kubernetes functional tests"
weight: 200
---

You can find the functional tests under `./test/functional/kubernetes/`. A functional tests (in our terminology) is a test that interacts with real hosting enviroments (Azure, Kubernetes), deploys real applications and resources, and covers realistic or simulated user scenarios.

## Running via GitHub workflow

These tests automatically run for every PR in the `build.yaml` github workflow.

The Kubernetes functional tests leverage KinD to create a kubernetes cluster for tests.

### How this works 

For each PR we run the following set of steps:

- Create a KinD Cluster.
- Add CRDs (Custom Resource Definitions) to the kubernetes cluster `make controller-install`
- Run `rad env init kubernetes` to add kubernetes to the `config.yaml`
- Deploy the radius kubernetes controller to the cluster `make controller deploy existing`
- Run deployment tests.

Some notes about how/why we do this:

- We want to ensure we're testing environment setup regularly but don't want to make PRs wait synchronously. If one of these async workflows fails, it will open an issue.

## Configuration

These tests use your local Kubernetes context and cluster. In a GitHub workflow, our automation makes the CI environment resemble a local dev scenario.

The tests use our product functionality (the Radius config file) to configure the environment.

## Running the tests locally

1. Create a Kubernetes cluster (KinD, AKS, etc.)
1. Add CRDs to the cluster with `make controller-install`
1. Build and install the controller to the cluster by running `make controller-deploy`. 
1. Place `rad` on your path
1. Make sure `rad-bicep` is downloaded (`rad bicep download`)
1. Add the kubernetes configuration to your config.yaml file by running `rad env init kubernetes`.
1. Run:

    ```sh
    make test-functional-kubernetes
    ```

When you're running locally with this configuration, the tests will use your locally selected Radius environment and your local copy of `rad`.

You can also run/debug individual tests from VSCode.

### Seeing log output

Some of these tests take a few minutes to run since they interact with cloud resources. You should configure VSCode to output verbose output so you can see the progress.

Open your VSCode `settings.json` with the command `Preferences: Open Settings (JSON)` and configure the following options:
```
{
    ...
    "go.testTimeout": "60m",
    "go.testFlags": [
        "-v"
    ]
}
```
