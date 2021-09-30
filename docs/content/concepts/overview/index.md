---
type: docs
title: Overview of Project Radius vision
linkTitle: Overview
description: An overview of Project Radius long-term vision and offerings
weight: 100
---

## State of cloud-native app development and management

- Developers do not have a common concept of an "Application".
- Neither Azure nor Kubernetes have an application "resource".
- Developers are being asked to deploy and manage apps on serverless, managed infra, on-premises, and across clouds.
- Developers need to be infra ops specialists, when many of the provisioning requirements can be automated.

## Mission statement

{{% alert title="Project Radius" color="primary" %}}
An intelligent application model that empowers developers to easily deploy and manage applications with a serverless experience.
{{% /alert %}}

Radius is an industry standard for microservices application deployment and management. It aims to be:
- A community developed open-source project
- The standard first-class managed application concept in Azure
- Loved by developers building applications

## Vision pillars

The Project Radius mission has the following pillars:

- **Make application composition simple**: Radius applications describe the whole application from the developer's point of view. This includes your code as well as supporting infrastructure like databases and API gateways.
- **Enable portable apps**: Radius applications describe the requirements and intentions of the code, not the exact configuration of the infrastructure. The Radius toolset (rad, Bicep, and VS Code tools) work the same way across different hosting platforms.
- **Add intelligence at every level**: Radius codifies and automates best-practices based on developer intentions. The Radius control plane has the intelligence to perform common operations like managing permissions and secrets.

## Platform strategy

Conceptually Radius can and will support all hosting platforms, from major public clouds, to Kubernetes on Raspberry Pi, to IoT and edge devices. We don't want to make assumptions as part of the model and user experience that limit our future options.

Our focus is on delivering good support for the following platforms:

- [Local development]({{< ref local >}}) as part of a developer inner-loop
- [Microsoft Azure]({{< ref azure>}}) as a managed-application serverless PaaS
- [Kubernetes]({{< ref kubernetes >}}) in all flavors and form-factors

## Next steps

{{< button text="Learn about the app model" page="appmodel-concept" >}}