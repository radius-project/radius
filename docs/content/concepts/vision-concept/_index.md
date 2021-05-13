---
type: docs
title: Overview of Project Radius vision
linkTitle: Radius vision
description: An overview of Project Radius long-term vision and offerings
weight: 50
---

- Radius is an industry standard for microservices application deployment and management
- Radius is a community developed open-source project
- Radius is the first-class managed application concept in Azure
- Radius is loved by developers building applications 

## Mission

> Radius is an intelligent application model that empowers developers to easily deploy and manage applications with a serverless experience. Radius significantly improves developer productivity, application reliability and time to market delivery.

This mission has the following pillars:

- Application Model: Radius applications describe the whole application from the developer's point of view. This includes compute resources (things that run your code) as well as supporting infrastructure like databases and API gateways.
- Portability: Radius applications describe the requirements and intentions of the code, not the exact configuration of the infrastructure. The Radius toolset (rad, Bicep, and VS Code tools) work the same way across different hosting platforms.
- Intelligence: Radius codifies and automates best-practices based on developer intentions. The Radius control plane has the intelligence to perform common operations like managing permissions and secrets.

## Platform strategy

Conceptually Radius can and will support all hosting platforms, from major public clouds, to Kubernetes on Raspberry Pi, to IoT and edge devices. We don't want to make assumptions as part of the model and user experience that limit our future options.

For now our focus is on delivering good support for the following platforms:

- Azure cloud as a managed *application* serverless PaaS
- Kubernetes in all flavors and form-factors
- Local development with Docker as part of a developer inner-loop