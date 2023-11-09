# Radius

Radius is a cloud-native application platform that enables developers and the platform engineers that support them to collaborate on delivering and managing cloud-native applications that follow organizational best practices for cost, operations and security, by default. Radius is an open-source project that supports deploying applications across private cloud, Microsoft Azure, and Amazon Web Services, with more cloud providers to come.

## Overview

The evolution of cloud computing has accelerated innovation for many companies, whether they are developing 2 or 3-tier applications or complex microservice-based applications. Cloud-native technologies like Kubernetes have made it easier to build applications that can run anywhere. However, as applications have grown in complexity, managing them in the cloud has become increasingly challenging. Companies now build cloud-native applications composed of interconnected services and deploy them to multiple public clouds and private infrastructure. While Kubernetes is a key enabler, organizations often need to create abstractions over Kubernetes to address its limitations. These abstractions usually focus on compute and aim to resolve Kubernetes' challenges, such as its lack of a formal application definition, mixing of infrastructure and application concepts, and overwhelming complexity. Developers also realize that their applications need more than just Kubernetes; they require support for dependencies like API front ends, key/value stores, caches, and observability systems. Amidst these challenges, corporate IT must enforce corporate standards, compliance, and security requirements while facilitating rapid application innovation.

Radius was designed to address these distinct but related challenges that arise across development and operations as companies continue their journey to cloud.  Radius meets application teams where they are by supporting proven technologies like Kubernetes, existing infrastructure tools including Terraform and Bicep and by integrating with existing CI/CD systems like GitHub Actions. Radius supports multi-tier web-plus-data to complex microservice applications like eShop, a popular cloud reference application from Microsoft.

**Key Features of the Radius Platform**

- *Team Collaboration*: Radius Applications and Environments enable developers to work with Operations on application definition and delivery.
- *Infrastructure Recipes*: Swappable infrastructure that complies with organization best practices and IT policy be default.
- *Application Graph*: Gain insights into the interconnections between services and infrastructure elements within an application.
- *Cloud Neutrality*: Deploy across development, on-premises and cloud environments with a consistent experience.
- *Incremental Adoption*: Seamlessly integrate Radius into existing workflows and Infrastructure-as-Code templates.

## Release Status

This is an early release of Radius which enables the community to learn about and experiment with the platform. Please let us know what you think and open Issues when you find bugs or want to request a new feature. Radius is not yet ready for production workloads.

## Getting Started

1. Follow the [Getting Started Guide](https://docs.radapp.io/getting-started/) to install and explore Radius.
2. Explore the [Tutorials](https://docs.radapp.io/tutorials) and [User Guides](https://docs.radapp.io/guides) to dive deeper into Radius and begin optimizing your applications.

## Getting Help

- ❓ **Have a Question?** - Visit our [Discord server](https://discord.gg/SRG3ePMKNy) to post your query, and we'll respond promptly.
- ⚠️ **Found an Issue?** - Refer to our [Contributing Issues guide](docs/contributing/contributing-issues) for instructions on filing a bug report.
- 💡 **Have a Proposal?** - Find out how to file a feature request in our [guide](docs/contributing/contributing-issues).

## Community

We welcome your contributions and suggestions! One of the easiest ways to contribute is to participate in Issue discussions, chat on [Discord server](https://discord.gg/SRG3ePMKNy) or the monthly [community calls](#community-calls). For more information on the community engagement, developer and contributing guidelines and more, head over to the [Radius community repo](https://github.com/radius-project/community).

### Contact Us

Please reach out to us via our [Discord server](https://discord.gg/SRG3ePMKNy), and we'll respond as soon as possible.

### Community calls

Every month, we host community calls to showcase new features, discuss upcoming milestones, and engage in Q&A sessions. Everyone is welcome! Visit our [community meetings page](https://github.com/radius-project/community/#community-meetings) for upcoming dates and meeting links.

## Contributing to Radius

- Visit [Contributing](./CONTRIBUTING.md) for more information on how to contribute to Radius.
- To author Radius Recipes visit [Author Custom Radius Recipes](https://docs.radapp.io/guides/recipes/howto-author-recipes/).
- To contribute to Radius documentation visit [Radius documentation](https://docs.radapp.io/contributing/docs/)

## Repositories

[Radius](https://github.com/radius-project/radius) is the primary repository for Radius, containing all code and documentation. In addition, we maintain the following repositories:

| Repo | Description |
|:-----|:------------|
| [Docs](https://github.com/radius-project/docs) | This repository contains the Radius documentation source for Radius.
| [Samples](https://github.com/radius-project/samples) | This repository contains the source code for quickstarts, reference apps, and tutorials for Radius.
| [Recipes](https://github.com/radius-project/recipes) | This repository contains commonly used Recipe templates for Radius Environments.
| [Website](https://github.com/radius-project/website) | This repository contains the source code for the Radius website.
| [Bicep](https://github.com/radius-project/bicep) | This repository contains the source code for Bicep, a DSL for deploying cloud resource types. 
| [AWS Bicep Types](https://github.com/radius-project/bicep-types-aws) | This repository contains the tooling for Bicep support for AWS resource types.

## Security

Please refer to our guide on [Reporting security vulnerabilities](SECURITY.md).

## Code of Conduct

Please refer to our [Radius Community Code of Conduct](https://github.com/radius-project/community/blob/main/CODE-OF-CONDUCT.md).
