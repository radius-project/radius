---
type: docs
title: "Radius resources"
linkTitle: "Resources"
description: "Learn how to model your Application's pieces with Radius resources"
weight: 200
---

Resources describe the code, data, and infrastructure pieces of an application.

Each node of an architecture diagram would map to one Resource. Together, an Application's Resources capture all of the important behaviors and requirements needed for a runtime to host that app. 

## Resource definition

In your app's Bicep file, a resource captures: 

| Property | Description | Example |
|----------|-------------|---------|
| **Resource type** | What type of thing is this? | `Container`
| **Name** | The logical name of the Resource, must be unique per-Application and resource type | `my-container`
| **Essentials** | How do I run this? | Container image and tag (`my-container:latest`)
| **Connections** | What other Resource will I interact with? | Need to read from `my-db` 
| **Routes** | What capabilities do I provide for others? | Offer an HTTP endpoint on `/home`
| **Traits** | What operational behaviors do I offer and interact with? | Need a Dapr sidecar (`dapr.io.App`)

### Examples

The following examples shows two Resources, one representing a [Container]({{< ref container >}}) and the other describing a [Dapr State Store](https://docs.dapr.io/developing-applications/building-blocks/state-management/state-management-overview/).

#### Container

{{< rad file="snippets/app.bicep" embed=true marker="//CONTAINER" >}}

#### Dapr State Store

{{< rad file="snippets/app.bicep" embed=true marker="//STATESTORE" >}}

Other resources, like the `storefront` container above, can now connect to this Dapr State Store and save/get state items.

## Services 

A dev team's app code will likely center around core runnable resources, which we call services. Running code can be modeled with services like a container or an App Service. [Learn more]({{< ref services >}})

## Connecting to resources 

There are several ways a service resource (like a container) can connect to other supporting resources. 

{{< cardpane >}}

{{< card header="**Direct Connection**" >}}
[<img src="direct-icon.png" alt="Connectors" style="width:325px"/>]({{< ref connectors >}})
Connect directly to Kubernetes({{< ref kubernetes-resources >}}) and Azure ({{< ref azure-resources >}}) resources. 

[Learn more]({{< ref connectors >}})
{{< /card >}}

{{< card header="**Connectors**" >}}
[<img src="connectors.png" alt="Connectors" style="width:325px"/>]({{< ref connectors >}})
Add portability to your application through platform-agnostic resources.
[Learn more]({{< ref connectors >}})
{{< /card >}}

{{< card header="**Custom/3rd Party**" >}}
<img src="custom.png" alt="Custom" style="width:300px"/>
Model and connect to external 3rd party resources<br /><br />
Coming soon!
{{< /card >}}
{{< /cardpane >}}


## Next step

{{< button text="Connections" page="connections-model.md" >}}

