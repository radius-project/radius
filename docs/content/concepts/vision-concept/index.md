---
type: docs
title: Overview of Project Radius vision
linkTitle: Radius vision
description: An overview of Project Radius long-term vision and offerings
weight: 100
---

Radius is an industry standard for microservices application deployment and management. It aims to be:
- A community developed open-source project
- The standard first-class managed application concept in Azure
- Loved by developers building applications

## Mission

{{% alert title="Project Radius mission" color="primary" %}}
Improve developer productivity, application reliability and time to market delivery, through intelligent application model that empowers developers to easily deploy and manage applications and dependencies with a serverless experience.
{{% /alert %}}

This mission has the following pillars:

- **Make application composition simple**: Radius applications describe the whole application from the developer's point of view. This includes your code as well as supporting infrastructure like databases and API gateways.
- **Enable portable apps**: Radius applications describe the requirements and intentions of the code, not the exact configuration of the infrastructure. The Radius toolset (rad, Bicep, and VS Code tools) work the same way across different hosting platforms.
- **Add intelligence at every level**: Radius codifies and automates best-practices based on developer intentions. The Radius control plane has the intelligence to perform common operations like managing permissions and secrets.

## Platform strategy

Conceptually Radius can and will support all hosting platforms, from major public clouds, to Kubernetes on Raspberry Pi, to IoT and edge devices. We don't want to make assumptions as part of the model and user experience that limit our future options.

For now our focus is on delivering good support for the following platforms:

- [Microsoft Azure]({{< ref azure-environments >}}) as a managed-application serverless PaaS
- [Kubernetes]({{< ref kubernetes-environments >}}) in all flavors and form-factors
- Local development with Docker as part of a developer inner-loop
