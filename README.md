# Radius

Radius is a developer-centric cloud-native application platform.

The core of Radius is using the declarative **application model** to describe complete applications that can be managed and deployed with an intelligent control plane. Radius uses the [Bicep](https://github.com/azure/bicep) language as a file-format and infrastructure-as-code tool.

## Overview

We are a Cloud Native Computing Foundation (CNCF) incubation project.

At a high level, Radius contains
- Extensions to the Bicep language
- Application model that represents developer concepts with a declarative model
- Schemas for different compute and non-compute resource types
- CLI tools for deployment and management
- Multiple runtimes for application hosting
  - *For now* this is just Kubernetes

[ToDo] radius high level component diagram

## Goals

- Enable developers to easily model their multi cloud appications 
  - For now we support Azure and AWS
- Enable portability across clouds. developers cans easily model their appications once and use it to deploy their application to any public cloud. 
- Be community driven, open and vendor neutral
- Gain new contributors
- Be incrementally adoptable.

## How it works

Radius contains extensions to Bicep language, that enables developers to model applications. Radius Cli consumes this bicep file and communicates with 
Universal Control Plane to deploy the application. Univeral Control Plane in turn works with Radius Core and Link Resource Providers to deploy various Application resources.It also communicates with other Cloud Providers as neccessary to set the resources for an appication.  

## Features

## Getting started

Visit the Radius [docs site](https://radapp.dev/getting-started/) to get up and running with Radius in minutes.

## Quickstarts and Samples

* See the [quickstarts repository](https://docs.radapp.dev/getting-started/quickstarts/) for code examples that can help you get started with Radius.

## Community <remove>

We want your contributions and suggestions! One of the easiest ways to contribute is to participate in discussions on the mailing list, chat on IM or the bi-weekly community calls.
For more information on the community engagement, developer and contributing guidelines and more, head over to the [Radius community repo](https://docs.radapp.dev/community/)


### Contact Us

[ToDo]

### Community Calls

Every two weeks we host a community call to showcase new features, review upcoming milestones, and engage in a Q&A. All are welcome!

📞 Visit https://docs.radapp.dev/community/#community-meetings for upcoming dates and the meeting link.

### Videos and Podcasts

[Todo]

## Contributing to Radius

See the [Development Guide](https://docs.radapp.dev/contributing/) to get started with building and developing.

## Repositories

| Repo | Description |
|:-----|:------------|
| [Radius](https://github.com/project-radius/radius) | The main repository that you are currently in. Contains the Radius code and overview documentation.
| [Docs](https://github.com/project-radius/radius) | The main repository that you are currently in. Contains the Radius code and overview documentation.
| [Samples](https://github.com/project-radius/radius) | The main repository that you are currently in. Contains the Radius code and overview documentation.
| [Receipes](https://github.com/project-radius/radius) | The main repository that you are currently in. Contains the Radius code and overview documentation.
| [Website](https://github.com/project-radius/radius) | The main repository that you are currently in. Contains the Radius code and overview documentation.
| [Bicep](https://github.com/project-radius/radius) | The main repository that you are currently in. Contains the Radius code and overview documentation.
| [AWS Bicep Types](https://github.com/project-radius/radius) | The main repository that you are currently in. Contains the Radius code and overview documentation.


## Code of Conduct
[ToDo]