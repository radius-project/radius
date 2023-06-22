# Understanding Radius repo code organization

This document describes the high-level organization of code for the Radius repository. The goal of this document is to capture most of the important details, not every single thing will be described here.

In general you should ask for guidance before creating a new top-level folder in the repo or creating a new folder in `pkg/`. There is usually a better place to put something.

## Root folders

| Folder     | Description                                                                           |
| ---------- | --------------------------------------------------------------------------------------|
| `build/`   | Makefiles and scripts referenced from the root Makefile                               |
| `cmd/`     | Entry points for executables built in the repository                                  |
| `cadl/`    | Definitions for generating Radius swagger files.                                      |
| `deploy/`  | Assets used to package, deploy, and install Radius                                    |
| `docs/`    | All project documentation                                                             |
| `hack/`    | Utility code to generate Radius bicep types                                           | 
| `pkg/`     | The majority of the Go code powering Radius                                           |
| `swagger/` | OpenAPI Specification v2 files to describe the REST APIs of Radius resource providers |
| `test/`    | Integration and end-to-end tests                                                      |


## Pkg folders

| Folder            | Description                                                                             |
| ----------------- | --------------------------------------------------------------------------------------- |
| `algorithm/`      | General purpose algorithms and data structures                                          |
| `armrpc/`         | Implementation containing shared functionality and utility for all Radius RP            |
| `aws/`            | Utility code and library integrations for working with AWS                              |
| `azure/`          | Utility code and library integrations for working with Azure                            |
| `cli/`            | Implementation code for the `rad` CLI                                                   |
| `corerp/`         | Resource Provider implementation for `Applications.Core` resources                      |
| `daprrp/`         | Resource Provider implementation for `Applications.Dapr` resources                      |
| `datastoresrp/`   | Resource Provider implementation for `Applications.Datastores` resources                |
| `health/`         | The health monitor service                                                              |
| `kubernetes/`     | Utility code and library integrations for working with Kubernetes                       |
| `kubeutil/`       | Utility code and working with Kubernetes on client side                                 |
| `linkrp/`         | Resource Provider implementation for `Applications.Link` resources                      |
| `logging/`        | Utility code for Radius logging                                                         |
| `messagingrp/`    | Resource Provider implementation for messaging Resource Provider                        |
| `middleware/`     | Implementation for all Radius middleware                                                |
| `metrics/`        | Code generating Radius metrics                                                          |
| `profiler/`       | Code and configs for Radius profiler                                                    |
| `recipes/`        | Implementation for Radius Recipes                                                       |
| `resourcekinds/`  | Definition of Radius resources                                                          |
| `resourcemodels/` | Code for identifying Radius resources in underlying system                              |
| `rp/`             | Code shared by Application.Core and Application.Link rps                                |
| `sdk/`            | Code for interfacing with Radius as a client                                            |
| `to/`             | Code for pointer to value conversions                                                   |
| `trace/`          | Utility code for generating Radius traces                                               |
| `ucp/`            | Implementation of Universal Control Plane                                               |
| `validator/`      | OpenAPI spec loader and validator                                                       |
| `version/`        | Infrastructure for how to version the Radius implementations                            |

