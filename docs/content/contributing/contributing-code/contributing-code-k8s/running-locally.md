---
type: docs
title: "Running Radius Kubernetes controller locally"
linkTitle: "Run Kubernetes controller locally"
description: "How to get the Radius Kubernetes controller running locally"
weight: 20
---

## Running integration tests

To run controller integration tests locally, run:

```
make test-controller
```

This will:
- Install the controller tools via go install (see https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest).
- Run the controller with these tools.

Note, these tests don't actually run against a kubernetes cluster. Therefore, resources like pods will not be created, though services and deployments will be created.


## Debugging integration tests in VSCode

Running the integration/controller tests should work by just running run test/debug test in VSCode. By default, the tests require that setup-envtest, a tool to get the necessary components for controller tests, must be installed.

```bash
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
```

When invoking the test, you either will need to have the KUBEBUILDER_ASSETS environment variable set to the path of the binary directory, or it will be obtained by calling `setup-envtest` in the test itself. Note: the `setup-envtest` tool has some options hard coded including:

- arch == amd64 as it isn't support on M1 macs
- k8s-version == 1.19.x as 1.20.x+ currently doesn't work in tests
