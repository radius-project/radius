---
type: docs
title: "Project Radius application model"
linkTitle: "Application model"
description: An overview of how the Radius app model is structured
weight: 200
no_list: true
---

Project Radius provides a descriptive framework for cloud native applications and their requirements. 


## Deployable architecture diagrams

Cloud-native applications are often designed and described using lines-and-boxes architecture diagrams as the starting point.

{{< imgproc app-diagram Fit "700x500" >}}
<i>A simple example of an online app.</i>
{{< /imgproc >}}

These diagrams often include  
- the resources that run application code 
- "supporting" resources - like databases, messages queues, api gateways, and secret stores
- information about the relationship between resources - like protocols, settings, and permissions.

Radius provides a way for developers to translate human-understandable application diagrams into human-understandable application code. 

## Project Radius app model

In Radius, the following concepts are used to describe application architecture:

{{< cardpane >}}
{{< card header="[**Radius Application**](./application-model)" >}}
A Radius Application includes all software, compute, data, and infrastructure used by the app.
{{< /card >}}
{{< card header="[**Components**](./components-model)" >}}
Each node on the diagram maps to one Component and describe the code, data, and infrastructure pieces of an application.
{{< /card >}}
{{< /cardpane >}}
{{< cardpane >}}
{{< card header="[**Connections**](./connections-model)" >}}
Connections describe a logical unit of communication between Components and model the edges between nodes in an architecture diagram.
{{< /card >}}
{{< card header="[**Traits**](./traits-model)" >}}
A Trait is a piece of configuration that specifies an operational behavior. Once defined, a trait can be added to Component definitions.
{{< /card >}}
{{< /cardpane >}}
{{< cardpane width=52% >}}
{{< card header="[**Scopes**](./scopes-model)" >}}
A Scope is a shared piece of configuration that applies to one or more Components. It's the circle you draw around Components in your architecture diagram.
{{< /card >}}
{{< /cardpane >}}


## App-model language

Radius uses the [Bicep language](https://github.com/azure/bicep) as its file-format and structure. Bicep is an existing Microsoft language that offers:
- A high quality authoring experience with modules, loops, parametrization, and templating
- ARM Deployment Stacks as the declarative deployment/rollback mechanism
- Ability to punch through abstractions to platform when necessary
- Extensions to work with other providers (e.g. Kubernetes, Azure Active Directory, etc.)


{{< button page="application-model" text="Learn about Radius Applications" >}}

