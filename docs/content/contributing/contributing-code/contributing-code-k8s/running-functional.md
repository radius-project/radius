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

- Create a Kubernetes Cluster. We'd recommend AKS for now, we have seen some stress issues with KinD.
- Add CRDs (Custom Resource Definitions) to the kubernetes cluster via `make controller-crd-install`.
- Deploy the radius kubernetes controller to the cluster via `make controller-deploy`.
- Run deployment tests.

## Configuration

These tests use your local Kubernetes context and cluster. In a GitHub workflow, our automation makes the CI environment resemble a local dev scenario.

The tests use our product functionality (the Radius config file) to configure the environment.

## Running the tests locally

1. Create a Kubernetes cluster (KinD, AKS, etc.).
1. Add CRDs to the cluster via

    ```sh
    make controller-crd-install
    ```

1. Build and install the controller to the cluster by running

    ```sh
    make controller-deploy
    ```

1. Place `rad` on your path.
1. Make sure `rad-bicep` is downloaded (`rad bicep download`).
1. Install dapr into the cluster by running

   ```sh
   dapr init -k --wait
   ```

1. Run:

   ```sh
   make test-functional-kubernetes
   ```
=======
When you're running locally with this configuration, the tests will use your locally selected Radius environment and your local copy of `rad`.

You can also run/debug individual tests from VSCode.

### Running the controller as a process in VSCode

Instead of building and installing the Radius Kubernetes controller, you can run the controller locally as a standalone process and have it interact with the cluster accordingly.

1. Create a Kubernetes cluster (KinD, AKS, etc.).
1. Add CRDs to the cluster with `make controller-install`.
1. Place `rad` on your path.
1. Make sure `rad-bicep` is downloaded (`rad bicep download`).
1. Install dapr into the cluster by running `dapr init -k --wait`.
1. Add an override to the `rad` environment to point to the local API server running. The API server is running on http://localhost:7443 by default.

```
environment:
  default: justin-dev
  items:
    justin-dev:
      context: justin-dev
      kind: kubernetes
      namespace: default
      apiserverbaseurl: http://localhost:7443
```
1. Set the environment variables `SKIP_APISERVICE_TLS` and `SKIP_WEBHOOKS` to `true` to skip TLS validation and skip webhooks (or in VSCode).
1. Run the controller locally on command line or VSCode (cmd/radius-controller/main.go).
1. Run `make test-functional-kubernetes`

You can also debug the controller in VSCode by running the cmd/radius-controller/main.go file. A launch.json configuration:

```json
{
    "name": "Debug k8s controller",
    "type": "go",
    "request": "launch",
    "mode": "debug",
    "program": "${workspaceFolder}/cmd/radius-controller/main.go",
    "env": {
        "SKIP_WEBHOOKS": "true", // Don't enable webhooks when running locally as they require a cert.
        "SKIP_APISERVICE_TLS": "true" // Don't enable TLS when running locally as it requires a cert.
    }
},
```

### Cleanup 

To cleanup the Kubernetes cluster, you'll need to do the following:

- `kubectl delete all`
- `helm uninstall radius -n radius-system`
- You can verify that all the resources are deleted by running `kubectl get all -A` and verifying that all resources are cleaned up.

Alternatively, if you are using KinD, you can delete the cluster with `kind delete cluster`
