# Understanding Radius repo code organization

This document describes the high-level organization of code for the Radius repository. The goal of this document is to capture most of the important details, not every single thing will be described here.

In general you should ask for guidance before creating a new top-level folder in the repo or creating a new folder in `pkg/`. There is usually a better place to put something.

## Root folders

| Folder     | Description                                                  |
| ---------- | ------------------------------------------------------------ |
| `build/`   | Makefiles and scripts referenced from the root Makefile      |
| `cmd/`     | Entry points for executables built in the repository         |
| `deploy/`  | Assets used to package, deploy, and install Radius           |
| `docs/`    | All project documentation                                    |
| `pkg/`     | The majority of the Go code powering Radius                  |
| `schemas/` | The schemas used to describe Radius types such as Components |
| `test/`    | Integration and end-to-end tests                             |


## Pkg folders

| Folder            | Description                                                                             |
| ----------------- | --------------------------------------------------------------------------------------- |
| `algorithm/`      | General purpose algorithms and data structures                                          |
| `azure/`          | Utility code and library integrations for working with Azure                            |
| `cli/`            | Utility code for the `rad` CLI                                                          |
| `handlers/`       | Resource handler implmentations                                                         |
| `health/`         | The health monitor service                                                              |
| `healthcontract/` | Data types for interfacing between the health monitor service and the resource provider |
| `hosting/`        | The hosting model for the RP process                                                    |
| `keys/`           | Azure tag and Kubernetes label constants                                                |
| `kubernetes/`     | Utility code and library integrations for working with Kubernetes                       |
| `model/`          | The data-driven model for representing the set of Radius types                          |
| `ucplogger/`      | Logging infrastructure                                                                  |
| `renderers/`      | Renderers for component implementations                                                 |
| `version/`        | Infrastructure for how to version the Radius implementations                               |
| `workloads/`      | Data types for renderers                                                                |