# Bicep Extensibility Types for Radius

## Status

accepted

## Context

[Bicep Extensibility](https://github.com/Azure/bicep/issues/3565) is a feature delivered by the ARM team which allows for resource to be deployed outside of the ARM control plane. We would like to adopt bicep extensibility for radius types to integrate in a more modular way with the Bicep project and reduce the need to keep a separate Bicep fork.

Since the extensibility features in Bicep at the time of writing are in the very early design and implementation steps, we were unable to completely reduce the need to keep a separate Bicep fork,due to the following blocker:

1. The mechanism by which extensibility types are loaded to the Bicep binaries is via a statically linked dll, as a consequence every time the extensibility types definition changes the Bicep solution requires compilation which in turn creates a tight coupling between Bicep and the Extensibility Provider components

## Decision

* We will maintain a Bicep fork with the necessary plumbing to support Radius types
* We will implement guardrails to ensure generated types are kept up to date whenever a change in the specification is committed to the codebase
* We will automate the process of publishing the Bicep bits so that they are accessible in both production and developer iterations

## Consequences

* We will maintain a copy of the type generator source in the Radius repository until the type generator is released and supported separately by the Bicep team. This code lives under `/hack/**`
* We will create and use the [`bicep-extensibility`](https://github.com/project-radius/bicep/tree/bicep-extensibility) branch to contain the code necessary to support Radius types as extensibility types in Bicep
* Whenever the type definitions for the resource provider are updated, the following must take place:
    1. Generate a set of markdown and json files that describe the type definitions in the Bicep internal serialization format (`make generate`)
    1. Verify the changes had the desired effect by inspecting the artifacts in the [project-radius/bicep](https://github.com/project-radius/bicep/tree/bicep-extensibility) repo under branch `bicep-extensibility` provide the desired functionality
    1. Merge the automatically generated PR, this will result with the generated Bicep bits to be published in the [radius blob storage](https://radiuspublic.blob.core.windows.net)
