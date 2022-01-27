---
type: docs
title: "Project Radius application model"
linkTitle: "Application model"
description: An overview of how the Radius app model is structured
weight: 200
no_list: false
---

Project Radius provides a descriptive framework for cloud native applications and their requirements. 

## Deployable architecture diagrams

Cloud-native applications are often designed and described using lines-and-boxes architecture diagrams as the starting point.

<!-- TODO: make this diagram match the app in the mockup below-->
{{< imgproc app-diagram Fit "700x500" >}}
<i>An example app represented as a block diagram.</i>
{{< /imgproc >}}

These diagrams often include:
- Infrastructure resources, including databases, messages queues, API gateways, and secret stores
- Services that run application code, such as containers.
- Relationships between resources, like protocols, settings, and permissions

Project Radius provides a way for developers to translate human-understandable application diagrams into human-understandable application code. 

## Applications as code

With the Radius app model, teams can easily codify and share pieces of a large, shared application. 
{{< rad file="snippets/appmodel-concept.bicep" embed=true >}}

The result is no longer just a flat list of resources - it's a fully fledged diagram of how the pieces relate to each other.
{{< imgproc ui-mockup-basic Fit "700x500">}}
<i>An example app represented in the Radius Azure Service.</i>
{{< /imgproc >}}

In fact, we're committed to creating a dev experience users love. So developers will be able to debug and iterate on that same app definition locally via VSCode as well. 
<!-- TODO: make all these diagrams & code show the identically same app -->
{{< imgproc vscode-mockup-basic Fit "700x500">}}
<i>An example app represented in VSCode.</i>
{{< /imgproc >}}


## App model language

Radius uses the [Bicep language](https://github.com/azure/bicep) as its file-format and structure. Bicep is an existing Microsoft language that offers:
- A high quality authoring experience with modules, loops, parametrization, and templating
- ARM Deployment Stacks as the declarative deployment/rollback mechanism
- Ability to punch through abstractions to platform when necessary
- Extensions to work with other providers (e.g. Kubernetes, Azure Active Directory, etc.)

## App model pieces

Learn more about the Radius app model pieces in the following docs:

