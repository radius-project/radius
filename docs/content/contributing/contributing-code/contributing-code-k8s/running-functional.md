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
- Add CRDs (Custom Resource Definitions) to the kubernetes cluster via `make controller-crd-install`.
- Run `rad env init kubernetes` to initialize the kubernetes environment in `config.yaml`.
- Deploy the radius kubernetes controller to the cluster via `make controller-deploy-existing`.
- Run deployment tests.

## Configuration

These tests use your local Kubernetes context and cluster. In a GitHub workflow, our automation makes the CI environment resemble a local dev scenario.

The tests use our product functionality (the Radius config file) to configure the environment.

## Running the tests locally

1. Create a Kubernetes cluster (KinD, AKS, etc.).
1. Add CRDs to the cluster via `make controller-crd-install`.
1. Build and install the controller to the cluster by running `make controller-deploy`. 
1. Place `rad` on your path.
1. Make sure `rad-bicep` is downloaded (`rad bicep download`).
1. Add the kubernetes configuration to your config.yaml file by running `rad env init kubernetes`.
1. Install dapr into the cluster by running `dapr init -k --wait`.
1. Run: `make test-functional-kubernetes`

When you're running locally with this configuration, the tests will use your locally selected Radius environment and your local copy of `rad`.

You can also run/debug individual tests from VSCode.

### Running the controller as a process in VSCode

Instead of building and installing the Radius Kubernetes controller, you can run the controller locally as a standalone process and have it interact with the cluster accordingly.

1. Create a Kubernetes cluster (KinD, AKS, etc.).
1. Add CRDs to the cluster with `make controller-install`.
1. Run the controller locally on command line or VSCode (cmd/radius-controller/main.go).
1. Place `rad` on your path.
1. Make sure `rad-bicep` is downloaded (`rad bicep download`).
1. Add the kubernetes configuration to your config.yaml file by running `rad env init kubernetes`.
1. Install dapr into the cluster by running `dapr init -k --wait`.
1. Run `make test-functional-kubernetes`

### Cleanup 

To cleanup the Kubernetes cluster, you'll need to do the following:
- `kubectl delete all`
- `helm uninstall radius -n radius-system`
- You can verify that all the resources are deleted by running `kubectl get all -A` and verifying that all resources are cleaned up.

Alternatively, if you are using KinD, you can delete the cluster with `kind delete cluster`
