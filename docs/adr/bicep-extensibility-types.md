# Bicep Extensibility Types for Radius

## Status

accepted

## Context

[Bicep Extensibility](https://github.com/Azure/bicep/issues/3565) is a feature delivered by the ARM team which allows for resource to be deployed outside of the ARM control plane. We would like to adopt bicep extensibility for radius types to integrate in a more modular way with the Bicep project and reduce the need to keep a separate Bicep fork.

The extensibility features in Bicep at the time of writing are in the very early design and implementation steps, the initial design is discussed in [the Bicep extensibility proposal](https://github.com/Azure/bicep/issues/3565). This design calls out two separate points of integration of extensibility types into the Bicep project: (1) Client-Side and (2) Server-Side (the linked proposal describes in them in detail).

The initial exploration of the Client-Side integration revealed that the mechanism used by Bicep to load extensibility type definitions is to statically link a dll that loads them. Current extensibility loader implementations (for Az types and Kubernetes types) implement the [`ITypeLoader`](https://github.com/project-radius/bicep/tree/bicep-extensibility/src/Bicep.Types.Radius) interface and are published as a dll in a NuGet package that in turn is consumed by the Bicep project (see here for the [exact code reference](https://github.com/project-radius/bicep/blob/f2e9572e7a0647bc13710f36cc7d6ef6da45dd48/src/Bicep.Core/Bicep.Core.csproj#L36-L38))  

In addition to the dll, other code changes are necessary on the Bicep solution before the types become available as listed below:

* The `BICEP_IMPORTS_ENABLED_EXPERIMENTAL` and `BICEP_SYMBOLIC_NAME_CODEGEN_EXPERIMENTAL` flags must be set in the process running Bicep (language server and cli)
* The following additions must be made to `Bicep.Core` project:
  * Author and connect a C# class (see here for a [concrete example](https://github.com/project-radius/bicep/blob/bicep-extensibility/src/Bicep.Core/Semantics/Namespaces/RadiusNamespaceType.cs)) that implements the `INamespaceProvider` interface in the `Bicep.Core.Semantics.Namespaces` namespace.
  * Author and connect a C# class (see here for a [concrete example](https://github.com/project-radius/bicep/blob/bicep-extensibility/src/Bicep.Core/TypeSystem/Radius/RadiusResourceTypeProvider.cs)) that implements the `IResourceTypeProvider` interface to load the definitions from the statically linked dll.

Iteration on the extensibility types definition separate from the Bicep release process is not possible until the coupling issues described above are addressed. We will continue to need a Bicep fork until that time.

In summary, the following blocker items must be addressed before we are able to fully adopt Bicep extensibility types:

* The mechanism by which extensibility types are loaded to the Bicep binaries changes to be loaded dynamically vs a statically  reference
* No code additions are needed in the Bicep solution to load Bicep extensibility type definitions

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
