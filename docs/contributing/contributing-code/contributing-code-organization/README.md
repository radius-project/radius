# Understanding Radius repo code organization

## Purpose

This document is a reference map of how code is organized in the Radius repository — the top-level folders and the packages under `pkg/` — so contributors can find where a given piece of functionality lives. It captures the important structure rather than every file. When in doubt about where new code belongs, ask for guidance before creating a new top-level folder or a new folder in `pkg/`; there is usually a better place to put something.

This document describes the high-level organization of code for the Radius repository. The goal of this document is to capture most of the important details, not every single thing will be described here.

## Root folders

| Folder      | Description                                                                           |
|-------------|---------------------------------------------------------------------------------------|
| `build/`    | Makefiles and scripts referenced from the root Makefile                               |
| `cmd/`      | Entry points for executables built in the repository                                  |
| `typespec/` | Definitions for generating Radius swagger files.                                      |
| `deploy/`   | Assets used to package, deploy, and install Radius                                    |
| `docs/`     | All project documentation                                                             |
| `hack/`     | Utility code to generate Radius bicep types                                           |
| `pkg/`      | The majority of the Go code powering Radius                                           |
| `swagger/`  | OpenAPI Specification v2 files to describe the REST APIs of Radius resource providers |
| `test/`     | Integration and end-to-end tests                                                      |

## Pkg folders

| Folder               | Description                                                                                                                                                                                |
|----------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `algorithm/`         | General purpose algorithms and data structures                                                                                                                                             |
| `armrpc/`            | Implementation containing shared functionality and utility for all Radius RP                                                                                                               |
| `aws/`               | Utility code and library integrations for working with AWS                                                                                                                                 |
| `azure/`             | Utility code and library integrations for working with Azure                                                                                                                               |
| `cli/`               | Implementation code for the `rad` CLI                                                                                                                                                      |
| `components/`        | Components and its folders hold the implementations of shared components used by the Radius control-plane services                                                                         |
| `controllers/`       | Kubernetes controllers for Radius                                                                                                                                                          |
| `corerp/`            | Resource Provider implementation for `Applications.Core` resources                                                                                                                         |
| `daprrp/`            | Resource Provider implementation for `Applications.Dapr` resources                                                                                                                         |
| `datastoresrp/`      | Resource Provider implementation for `Applications.Datastores` resources                                                                                                                   |
| `dynamicrp/`         | Implementation of the dynamic resource provider. The dynamicrp is responsible for managing the lifecycle of resources that are defined without their own resource provider implementation. |
| `kubernetes/`        | Utility code and library integrations for working with Kubernetes                                                                                                                          |
| `kubeutil/`          | Utility code and working with Kubernetes on client side                                                                                                                                    |
| `portableresources/` | Shared Resource Provider implementation for portable resources                                                                                                                             |
| `logging/`           | Utility code for Radius logging                                                                                                                                                            |
| `messagingrp/`       | Resource Provider implementation for `Applications.Messaging` resources                                                                                                                    |
| `middleware/`        | Implementation for all Radius middleware                                                                                                                                                   |
| `metrics/`           | Code generating Radius metrics                                                                                                                                                             |
| `profiler/`          | Code and configs for Radius profiler                                                                                                                                                       |
| `recipes/`           | Implementation for Radius Recipes                                                                                                                                                          |
| `rp/`                | Code shared by multiple rps                                                                                                                                                                |
| `sdk/`               | Code for interfacing with Radius as a client                                                                                                                                               |
| `to/`                | Code for pointer to value conversions                                                                                                                                                      |
| `trace/`             | Utility code for generating Radius traces                                                                                                                                                  |
| `ucp/`               | Implementation of Universal Control Plane                                                                                                                                                  |
| `validator/`         | OpenAPI spec loader and validator                                                                                                                                                          |
| `version/`           | Infrastructure for how to version the Radius implementations                                                                                                                               |
