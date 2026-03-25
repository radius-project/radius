# Planes APIs

* **Author**: `Ryan Nowak (@rynowak)`

## Overview

UCP provides a concept called *planes* that represent an instance of a resource management system like Kubernetes, Radius, AWS, or Azure. This is part of the UCP API and the concept of plane is present in every API call or result from UCP or Radius.

Since planes are instances, they can be dynamically manged by users through the API. For example, Radius is a multi-tenant system. When we configure Radius today we configure a single tenant, but we have the capability to register additional tenants.

This document proposes an improved design for the API representation of plane resources. This was one of the earliest features defined in UCP, and it sorely needs an update. The current design will not scale well in terms of complexity as we add additional types of planes. Since we have additional work planned here we should address the technical debt. 

## Terms and definitions

Please read the [architecture](https://docs.radapp.io/concepts/technical/architecture/) and [api](https://docs.radapp.io/concepts/technical/api/) documentation. The terms defined and explained there are referenced heavily.

## Objectives

### Goals

- Simplify the design of existing plane APIs. 
- Improve consistency between the plane APIs and other resource APIs by addressing gaps.
- Address technical debt.

### Non goals

- (out of scope) Add new plane types or functionality.

### User scenarios (optional)

This is a highly technical design proposal that affects the internals of UCP & Radius. If we do a good job users don't get exposed to these details.

## Design

The current design defines a single resource type that is used to represent all of aws, azure, and radius planes in the API. This is bad for scaling up the number of planes because all new plane types are a revision of the current API. 

This is inconsistent with how we do API design in general. An Azure plane and an AWS plan are different *things* with different behaviors, they should be separate. If we need to introduce validation that's unique for Azure planes, then it has to be in a shared code-path.

The proposal is to define separate resource types for each plane. If we were starting fresh today this is how we would do it.

The best reason to change is that the current design allows nonsense. There's nothing stopping you from using an AWS plane definition and registering it as an Azure plane. The API design says that's fine. 

### Design details

*This is primarily an API change. Beyond API changes the intention is to keep the behavior consistent.*

Currently the planes controllers are shared between all plane types, and there is a single datamodel and converter. In the proposed design we will continue to use the existing controllers, but a separate datamodel and converter per-plane-type.

This is actually a simplification for features like the Radius proxy, resource groups, and tracked resources. Because these are specific to Radius they can remove a few validation steps. They currently need to protect against error cases like an non-Radius plane being registered as a Radius plane.

### API design (if applicable)

#### Current Design

The current design for the plane resource looks like this:

```typespec
@doc("The plane resource")
model PlaneResource is TrackedResourceRequired<PlaneResourceProperties, "planes"> {
  @key("planeType")
  @doc("The plane type.")
  @segment("planes")
  @path
  name: string;
}

@doc("Plane kinds supported.")
enum PlaneKind {
  @doc("UCP Native Plane")
  UCPNative,

  @doc("Azure Plane")
  Azure,

  @doc("AWS Plane")
  AWS,
}

@doc("The Plane properties.")
model PlaneResourceProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;

  @doc("The kind of plane")
  kind: PlaneKind;

  @doc("URL to forward requests to for non UCP Native Plane")
  url?: string;

  @doc("Resource Providers for UCP Native Plane")
  resourceProviders?: Record<string>;
}
```

Some examples:

```json
{
  "id": "/planes/aws/aws",
  "name": "aws",
  "properties": {
    "kind": "AWS"
  }
}
```


```json
{
  "id": "/planes/azure/azurecloud",
  "name": "azurecloud",
  "properties": {
    "kind": "Azure",
    "url": "https://management.azure.com"
  }
}
```

```json
{
  "id": "/planes/radius/local",
  "name": "local",
  "properties": {
    "kind": "UCPNative",
    "resourceProvider": {
      "Applications.Core": "http://applicaitons-rp:8080""
    }
  }
}
```

This design leverages discriminated unions. The `type` and `location` fields are missing from the response due to bugs.

#### Proposed Design

There are 4 parts to the API new design, because we're separating out one type into 4.

- A "generic" plane resource that's used with `/planes` (list all planes of all types)
- AWS plane: `System.Aws/planes`
- Azure plane: `System.Azure/planes`
- Radius plane: `System.Radius/planes`

**Generic Plane**

This is only used for the `/planes` endpoint (read only).

Note: The resource type `System.Resource/planes` needs to be defined in code for infrastructure purposes. It's not visible to users.

```typespec
@doc("The generic representation of a plane resource")
model GenericPlaneResource
  is TrackedResourceRequired<GenericPlaneResourceProperties, "System.Resources/planes", "planes"> {
  @key("planeType")
  @doc("The plane type.")
  @segment("planes")
  @path
  name: string;
}

#suppress "@azure-tools/typespec-azure-core/bad-record-type"
@doc("The properties of the generic representation of a plane resource.")
model GenericPlaneResourceProperties {
  @doc("The status of the asynchronous operation.")
  @visibility("read")
  provisioningState?: ProvisioningState;
}

@armResourceOperations
interface Planes {
  @doc("List all planes")
  listPlanes is UcpResourceList<GenericPlaneResource, ApiVersionParameter>;
}
```

**AWS Plane**

The new resource type is `System.Aws/planes`. The AWS plane represents an "instance" of AWS. We refer to the main one as `aws`. 

This is the full definition. I'll omit the operations for the other plane types in the doc because they are the same with some names swapped.

Notably, we don't define any relevant settings for the AWS plane (properties just has provisioning state). 

```typespec
@doc("The AWS plane resource")
model AwsPlaneResource
    is TrackedResourceRequired<
        AwsPlaneResourceProperties,
        "System.Aws/planes",
        "aws"
    > {
    @doc("The plane name.")
    @segment("aws")
    @path
    @key("planeName")
    name: ResourceNameString;
}

@doc("The Plane properties.")
model AwsPlaneResourceProperties {
    @doc("The status of the asynchronous operation.")
    @visibility("read")
    provisioningState?: ProvisioningState;
}

@route("/planes")
@armResourceOperations
interface AwsPlanes {
    @doc("List AWS planes")
    @get()
    @route("/aws")
    @armResourceList(AwsPlaneResource)
    list(
        ...ApiVersionParameter,
    ): ArmResponse<ResourceListResult<AwsPlaneResource>> | ErrorResponse;

    @doc("Get a plane by name")
    get is UcpResourceRead<
        AwsPlaneResource,
        PlaneBaseParameters<AwsPlaneResource>
    >;

    @doc("Create or update a plane")
    createOrUpdate is UcpResourceCreateOrUpdateAsync<
        AwsPlaneResource,
        PlaneBaseParameters<AwsPlaneResource>
    >;

    @doc("Update a plane")
    update is UcpCustomPatchAsync<
        AwsPlaneResource,
        PlaneBaseParameters<AwsPlaneResource>
    >;

    @doc("Delete a plane")
    delete is UcpResourceDeleteAsync<
        AwsPlaneResource,
        PlaneBaseParameters<AwsPlaneResource>
    >;
}
```

**Azure Plane**

The new resource type is `System.Azure/planes`. The Azure plane represents an "instance" of Azure. We refer to the main one as `azurecloud`. 

The only setting we define for Azure is the URL of ARM. The URL is different for different instance of Azure.

```typespec
@doc("The Azure plane resource.")
model AzurePlaneResource
    is TrackedResourceRequired<AzurePlaneResourceProperties, "System.Azure/planes", "azure"> {
    @doc("The plane name.")
    @segment("azure")
    @path
    @key("planeName")
    name: ResourceNameString;
}

@doc("The Plane properties.")
model AzurePlaneResourceProperties {
    @doc("The status of the asynchronous operation.")
    @visibility("read")
    provisioningState?: ProvisioningState;

    @doc("The URL used to proxy requests.")
    url: string;
}
```

**Radius Plane**

The new resource type is `System.Radius/planes`. The Radius plane represents a tenant of Radius. We automatically create one called `local` when you initialize Radius.

The only setting we define for the Radius plane is the list of resource providers and their addresses. This will be revisited as part of the user-defined-types.

```typespec
@doc("The Radius plane resource.")
model RadiusPlaneResource
    is TrackedResourceRequired<RadiusPlaneResourceProperties, "System.Radius/planes"> {
    @doc("The plane name.")
    @segment("radius")
    @path
    @key("planeName")
    name: ResourceNameString;
}

@doc("The Plane properties.")
model RadiusPlaneResourceProperties {
    @doc("The status of the asynchronous operation.")
    @visibility("read")
    provisioningState?: ProvisioningState;

    @doc("Resource Providers for UCP Native Plane")
    resourceProviders: Record<string>;
}
```

## Alternatives considered

The status quo is not good. I'm working on adding the Kubernetes plane, and I didn't want to make this worse. I don't think keep the existing design with a discriminated union is a reasonable idea.

I've coded up a version of this proposal and it simplifies validation in the code that works with planes. This makes the design most consistent with our existing resource providers.

## Test plan

These features are tested by integration tests since they don't need to change any state outside of UCP. I'm expanding the integration tests to cover all of the CRUDL APIs for the new resource types. I'm also improving the consistency of these APIs by including fields like `tags`, `systemData`, and `location` that are present in the API definition but not implemented.

## Security

No changes to security model. 

## Compatibility (optional)

This is a breaking change. Fortunately the only consumer of these APIs is inside UCP so the impact is limited.

UCP initializes the "default planes" as part of its startup sequence, so it should "just work" for an existing Radius installation.

## Monitoring

No new monitoring needed. 

## Development plan

This is an API breaking change so it can't be staged. 

## Open issues

**Naming?**

We had the debate about naming and ended up choosing a style like `System.<technology>/planes`.
