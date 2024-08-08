# Radius

Radius is a cloud-native application platform that enables developers and the platform engineers that support them to collaborate on delivering and managing cloud-native applications that follow organizational best practices for cost, operations and security, by default. Radius is an open-source project that supports deploying applications across private cloud, Microsoft Azure, and Amazon Web Services, with more cloud providers to come. 

Radius is a [Cloud Native Computing Foundation (CNCF) sandbox project](https://www.cncf.io/sandbox-projects/).

## Overview

The evolution of cloud computing has increased the speed of innovation for many companies, whether they are building 2 and 3-tier applications, or complex microservice-based applications. Cloud native technologies like Kubernetes have made it easier to build applications that can run anywhere. At the same time, many applications have become more complex, and managing them in the cloud increasingly difficult, as companies build cloud-native applications composed of interconnected services and deploy them to multiple public clouds and their private infrastructure. While Kubernetes is a key enabler, we see many organizations building abstractions over Kubernetes, usually focused on compute, to work around its limitations:  Kubernetes has no formal definition of an application, it mingles infrastructure and application concepts and it is overwhelmingly complex.  Developers also inevitably realize their applications require much more than Kubernetes, including support for dependencies like application programming interface (API) front ends, key/value stores, caches, and observability systems.  Amidst these challenges for developers, their corporate IT counterparts also must enforce an ever-growing matrix of corporate standards, compliance, and security requirements, while enabling rapid application innovation. 

Radius was designed to address these distinct but related challenges that arise across development and operations as companies continue their journey to cloud.  Radius meets application teams where they are by supporting proven technologies like Kubernetes, existing infrastructure tools including Terraform and Bicep and by integrating with existing CI/CD systems like GitHub Actions. Radius supports multi-tier web-plus-data to complex microservice applications like eShop, a popular cloud reference application from Microsoft.

Key features of the Radius platform include: 
- *Team Collaboration*: Radius Applications and Environments allow developers to work with Operations on application definition and delivery.
- *Infrastructure Recipes*: Swappable infrastructure that complies with organization best practices and IT policy be default.
- *Application Graph*: Understand how services and infrastructure in an application are interconnected.
- *Cloud Neutral*: Deploy across development, on-premises and cloud environments with a consistent experience.
- *Incremental Adoption*: Integrate Radius into existing workflows and existing catalogs of Infrastructure-as-Code templates.

## Release status

This is an early release of Radius which enables the community to learn about and experiment with the platform. Please let us know what you think and open Issues when you find bugs or want to request a new feature. Radius is not yet ready for production workloads.

## Getting started

1. Follow the [getting started guide](https://docs.radapp.io/getting-started/) to install and try out Radius
1. Visit the [Tutorials](https://docs.radapp.io/tutorials) and [User Guides](https://docs.radapp.io/guides) to learn more about Radius and start radifying your apps

## Getting help

- ❓ **Have a question?** - Visit our [Discord server](https://discord.gg/SRG3ePMKNy) to post your question and we'll get back to you ASAP
- ⚠️ **Found an issue?** - Refer to our [Issues guide](docs/contributing/contributing-issues) for instructions on filing a bug report
- 💡 **Have a proposal?** - Refer to our [Issues guide](docs/contributing/contributing-issues) for instructions on filing a feature request

## Community

We welcome your contributions and suggestions! One of the easiest ways to contribute is to participate in Issue discussions, chat on [Discord server](https://discord.gg/SRG3ePMKNy) or the monthly [community calls](#community-calls). For more information on the community engagement, developer and contributing guidelines and more, head over to the [Radius community repo](https://github.com/radius-project/community).

### Contact us

Please visit our [Discord server](https://discord.gg/SRG3ePMKNy) to contact us and we'll get back to you ASAP.

### Community calls

Every month we host a community call to showcase new features, review upcoming milestones, and engage in a Q&A. All are welcome!

📞 Visit our [community meetings](https://github.com/radius-project/community/#community-meetings) page for upcoming dates and the meeting link.

## Contributing to Radius

Visit [Contributing](./CONTRIBUTING.md) for more information on how to contribute to Radius.
To author Radius Recipes visit [Author Custom Radius Recipes](https://docs.radapp.io/guides/recipes/howto-author-recipes/).
To contribute to Radius documentation visit [Radius documentation](https://docs.radapp.io/contributing/docs/)

## Repositories

[Radius](https://github.com/radius-project/radius) is the main Radius repository. It contains all of Radius code and documentation.
In addition, we have the below repositories.

| Repo | Description |
|:-----|:------------|
| [Docs](https://github.com/radius-project/docs) | This repository contains the Radius documentation source for Radius.
| [Samples](https://github.com/radius-project/samples) | This repository contains the source code for quickstarts, reference apps, and tutorials for Radius.
| [Recipes](https://github.com/radius-project/recipes) | This repo contains commonly used Recipe templates for Radius Environments.
| [Website](https://github.com/radius-project/website) | This repository contains the source code for the Radius website.
| [AWS Bicep Types](https://github.com/radius-project/bicep-types-aws) | This repository contains the tooling for Bicep support for AWS resource types.


## Security

Please refer to our guide on [Reporting security vulnerabilities](SECURITY.md)

## Code of conduct

Please refer to our [Radius Community Code of Conduct](https://github.com/radius-project/community/blob/main/CODE-OF-CONDUCT.md)
