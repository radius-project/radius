# Radius

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/radius-project/radius/badge)](https://scorecard.dev/viewer/?uri=github.com/radius-project/radius) [![OpenSSF Best Practices](https://www.bestpractices.dev/projects/11971/badge)](https://www.bestpractices.dev/projects/11971)

> [!NOTE]
> Integration with the GitHub Copilot app is now available via the [Radius Canvas](https://github.com/radius-project/ai-extensions).

Radius is an open source, cloud-native application platform that enables developers and the platform engineers that support them to collaborate on delivering and managing cloud-native applications that follow organizational best practices for cost, operations and security, by default. It enables developers and platform engineers to model an entire application — its services and the infrastructure they depend on (databases, message queues, caches, and more) — as a single, self-contained definition. Developers describe their application in a format that is portable across environments; platform engineers supply Recipes that provision that infrastructure with built-in compliance. Radius runs on Kubernetes and deploys applications across local, private cloud, Microsoft Azure, and Amazon Web Services environments, with more cloud providers to come.

Radius is a [Cloud Native Computing Foundation (CNCF) sandbox project](https://www.cncf.io/sandbox-projects/).

## Overview

A modern application is more than the containers that run on Kubernetes. It's a collection of services plus the infrastructure they depend on — datastores, caches, message brokers, API gateways, and observability — frequently spread across clusters and multiple clouds. Kubernetes has no first-class notion of an *application*: it blends infrastructure and application concerns, so teams end up hand-rolling their own abstractions to fill the gap.

Radius adds that missing application layer. It provides an explicit application model that captures your services, their dependencies, and the connections between them, so the whole app — not just individual containers — is deployed, visualized as an application graph, and managed as one unit. Developers author their application definitions and deploy them with the `rad` CLI; the Radius control plane provisions each dependency through a Recipe, which is a reusable infrastructure template (e.g., Terraform or Bicep) that platform engineers curate to enforce policy by default. Because Recipes are swappable per environment, the same application definition deploys unchanged from a developer's local cluster to production on Azure, AWS, or directly to a Kubernetes cluster.

Key features of the Radius platform include:

- *Team Collaboration*: Radius Applications and Environments allow developers to work with Operations on application definition and delivery.
- *Infrastructure Recipes*: Swappable infrastructure that complies with organization best practices and IT policy by default.
- *Application Graph*: Understand how services and infrastructure in an application are interconnected.
- *Cloud Neutral*: Deploy across development, on-premises and cloud environments with a consistent experience.
- *Incremental Adoption*: Integrate Radius into existing workflows and existing catalogs of Infrastructure-as-Code templates.

## Getting started

1. Follow the [getting started guide](https://docs.radapp.io/getting-started/) to install and try out Radius
1. Visit the [Tutorials](https://docs.radapp.io/tutorials) and [User Guides](https://docs.radapp.io/guides) to learn more about Radius and start radifying your apps

## Join us in building Radius

We are growing our community of adopters and contributors to help shape where Radius goes next. Try it out, tell us what you think, and open [Issues](https://github.com/radius-project/radius/issues/new/choose) when you find bugs or want to request a new feature. Early feedback has an outsized influence on the direction of the project.

## Getting help

- ❓ **Have a question?** - Visit our [Discord server](https://discord.gg/SRG3ePMKNy) to post your question and we'll get back to you ASAP
- ⚠️ **Found an issue?** - Refer to our [Issues guide](docs/contributing/contributing-issues) for instructions on filing a bug report
- 💡 **Have a proposal?** - Refer to our [Issues guide](docs/contributing/contributing-issues) for instructions on filing a feature request

## Community

We welcome your contributions and suggestions! One of the easiest ways to contribute is to participate in Issue discussions, chat on [Discord server](https://discord.gg/SRG3ePMKNy) or the quarterly [community calls](#community-calls). For more information on the community engagement, developer and contributing guidelines and more, head over to the [Radius community repository](https://github.com/radius-project/community).

### Contact us

Please visit our [Discord server](https://discord.gg/SRG3ePMKNy) to contact us and we'll get back to you ASAP.

### Community calls

Every quarter we host a community call to showcase new features, review upcoming milestones, and engage in a Q&A. All are welcome!

📞 Visit our [community meetings](https://github.com/radius-project/community/#community-meetings) page for upcoming dates and the meeting link.

## Contributing to Radius

Visit [Contributing](./CONTRIBUTING.md) for more information on how to contribute to Radius.

To author Radius Resource Types and Recipes, visit the [Resource Types and Recipes repository](https://github.com/radius-project/resource-types-contrib).

To contribute to Radius documentation, visit [Radius documentation](https://docs.radapp.io/contributing/docs/).

## Repositories

[Radius](https://github.com/radius-project/radius) is the main Radius repository. It contains all of Radius code and documentation.

In addition, we have the below repositories:

| Repository | Description |
|:-----|:------------|
| [AI Extensions](https://github.com/radius-project/ai-extensions) | This repository contains extensions and integrations with AI tools, including the Radius Canvas integration for the GitHub Copilot app.
| [Docs](https://github.com/radius-project/docs) | This repository contains the source for the Radius documentation.
| [Samples](https://github.com/radius-project/samples) | This repository contains the source code for quickstarts, reference apps, and tutorials for Radius.
| [Resource Types and Recipes](https://github.com/radius-project/resource-types-contrib) | This repository contains commonly used Resource Types and Recipe templates for Radius Environments.
| [Dashboard](https://github.com/radius-project/dashboard) | This repository contains the source code for the Radius dashboard.
| [Website](https://github.com/radius-project/website) | This repository contains the source code for the Radius website.
| [AWS Bicep Types](https://github.com/radius-project/bicep-types-aws) | This repository contains the tooling for Bicep support for AWS resource types.

## Security

Please refer to our guide on [Reporting security vulnerabilities](SECURITY.md)

## Code of conduct

Please refer to our [Radius Community Code of Conduct](https://github.com/radius-project/community/blob/main/CODE-OF-CONDUCT.md)
