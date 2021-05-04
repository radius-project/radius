---
type: docs
title: "Project Radius Documentation"
linkTitle: "Project Radius docs"
description: "Documentation on the Project Radius application model"
weight: 1
---

Project Radius is a developer-centric cloud-native application platform.

The core of Radius is using the declarative application model to describe complete applications that can be managed and deployed with an intelligent control plane. Radius uses the [Bicep language](https://github.com/Azure/bicep) as a file-format and infrastructure-as-code tool.

Radius consists of:

- Extensions to the Bicep language
- Application model that represent developer concepts with a declarative model
- Schemas for different compute and non-compute resource types
- CLI tools for deployment and management
- Multiple runtimes for application hosting
    - For now this is just Kubernetes

## Getting started

You can begin with Radius by downloading and installing the Radius CLI

{{< button text="Get Started 🚀" page="install-cli.md" >}}