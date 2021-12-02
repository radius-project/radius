---
type: docs
title: "Radius applications"
linkTitle: "Applications"
description: "Radius Applications as the top-level resource"
weight: 100
---

## Definition

The Radius Application contains everything on an app diagram. That includes all the compute, data, and infrastructure. 

<!-- TODO: expand this diagram to include more about the infra layer -->
{{< imgproc radius-application Fit "700x500">}}
<i>A Radius Application encompases all the containers, databases, and APIs for an app.</i>
{{< /imgproc >}}

### Authoring an Application

Applications are the top-level *resource*. The descriptions of the app's sub-components or external resources the app interacts with will be defined within a top-level `resource app` in a .bicep file: 
{{< rad file="snippets/blank-app.bicep" embed=true >}}

Currently, this example app is an empty shell and has no child resources defined.

It's up to the user to define what they consider part of the app. Generally, users should include both all the runnable components (things that run code) and all the non-runnable components (data and infrastructure resources).


### Referencing existing resources

Apps are often comprised of a combination of existing constant resources like databases and more ephemeral resources like containers that run code.   

Radius makes it easy for multiple teams to bring their own services or infrastructure into a single application alongside Radius-managed resources.   


{{< rad file="snippets/appmodel-existing.bicep" embed=true >}}



### Deploying an Application
A Radius Application is deployed by using the rad CLI to deploy the Bicep file containing the app. For example:

```sh
rad deploy example.bicep
```

This command deploys the app and either launches or connects to its child resources as needed.

<!-- TODO: high-level overview of managing an app -->

{{< button text="Radius Components" page="components-model.md" >}}
