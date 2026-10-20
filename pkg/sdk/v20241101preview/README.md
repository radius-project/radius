# Azure Container Instances SDK (2024-11-01-preview)

This package contains the Go client for the Azure Container Instances
`2024-11-01-preview` API surface (NGroups and Container Group Profiles) used by
the Radius ACI renderer and handlers.

## Maintenance

These sources were originally produced by AutoRest, but the project no longer
generates them. The `2024-11-01-preview` NGroups/CGProfile APIs are **not**
published in [`github.com/Azure/azure-sdk-for-go`](https://github.com/Azure/azure-sdk-for-go)
and there is no TypeSpec source for them, so there is no automated
code-generation step.

Treat the `*.go` files in this directory as **hand-maintained source**. Apply
API changes directly here. The per-file `Code generated ... DO NOT EDIT`
headers are retained only so that linters continue to treat these files as
generated; they no longer imply an active generator.

The original Swagger specification is retained for reference under
[`pkg/sdk/aci-specification`](../aci-specification).
