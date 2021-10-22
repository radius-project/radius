---
type: docs
title: "Project Radius application model"
linkTitle: "Application model"
description: An overview of Project Radius application model and what it provides to the user
weight: 200
no_list: true
---

Project Radius provides an *application model* - a descriptive framework for cloud native applications and their requirements. This section is a conceptual guide for how the Radius model is structured, and explains at high level the concepts at work when you describe and deploy an application with Radius.

## App-model language

Radius uses the [Bicep langauge](https://github.com/azure/bicep) as its file-format and structure. Bicep offers to the user:
- A high quality authoring experience with modules, loops, parametrization, and templating
- ARM Deployment Stacks as the declarative deployment/rollback mechanism
- Ability to punch through abstractions to platform when necessary
- Extensions to work with other providers (e.g. Kubernetes, Azure Active Directory, etc.)


## Deployable architecture diagrams

To start understanding Radius - think about how cloud-native applications are first designed. It's typical to create a *lines-and-boxes* architecture diagram as the starting point.

{{< imgproc app-diagram Fit "700x500" >}}
<i>A simple example of an online shopping app has a collection of connections and resources.</i>
{{< /imgproc >}}

An architecture diagram would include all of the pieces of the application both the components that run your code as well as other components like databases, messages queues, api gateways, secret stores, and more. These components make up the nodes of the diagram.

An architecture diagram would also include lines connecting the components of the application that indicate the flows of communication between components. These lines can be annotated with the essential information about these points of communication: 

- Protocols in use
- Settings like port numbers or hostnames
- The level of permissions needed
- and more.....

These lines make up the edges of the diagram, they describe the relationships between components.

## Project Radius app model

Radius offers a set of concepts that can be used to describe the application architecture. These concepts are:

{{< cardpane >}}
{{< card header="[**Application**](./application-model)" >}}
The Radius Application is the biggest possible circle you could draw around your software, including all the compute, data, and infrastructure.
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

{{< button page="application-model" text="Learn about the Application" >}}
