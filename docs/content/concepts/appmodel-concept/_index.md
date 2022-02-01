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

## App model language

Radius uses the [Bicep language](https://github.com/azure/bicep) as its file-format and structure. Bicep is an existing Microsoft language that offers:
- A high quality authoring experience with modules, loops, parametrization, and templating
- ARM Deployment Stacks as the declarative deployment/rollback mechanism
- Ability to punch through abstractions to platform when necessary
- Extensions to work with other providers (e.g. Kubernetes, Azure Active Directory, etc.)

## App model pieces

Learn more about the Radius app model pieces in the following docs:

