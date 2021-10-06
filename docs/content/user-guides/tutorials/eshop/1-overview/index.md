---
type: docs
title: "eShop on Containers tutorial overview"
linkTitle: "Overview"
slug: "overview"
description: "Learn about the eShop on Containers tutorial application"
weight: 100
---

## Background

The [eShop on Containers](https://github.com/dotnet-architecture/eShopOnContainers) reference application is a sample .NET Core reference application based on a simplified microservices architecture and Docker containers.

### Architecture

<img src="architecture.png" alt="Architecture diagram of eShop on containers" width=900 >

From the [eShop repo](https://github.com/dotnet-architecture/eShopOnContainers#architecture-overview):

> The architecture proposes a microservice oriented architecture implementation with multiple autonomous microservices (each one owning its own data/db) and implementing different approaches within each microservice (simple CRUD vs. DDD/CQRS patterns) using HTTP as the communication protocol between the client apps and the microservices and supports asynchronous communication for data updates propagation across multiple services based on Integration Events and an Event Bus.

### Deployment

<img src="deploy.png" alt="Screenshot of the list of steps to deploy eShop" width=250 >

Currently, [deploying eShop to Azure Kubernetes Services](https://github.com/dotnet-architecture/eShopOnContainers/wiki/Deploy-to-Azure-Kubernetes-Service-(AKS)) requires deploying a cluster and the backing infrastructure, configuring multiple CLIs and tools, running deployment scripts, and manually copying/pasting credentials and endpoints.

## Adding Radius

Adding Project Radius to the eShop on containers application will allow teams to:
- Define the entire collection of microservices and backing infrastructure as a single application
- Easily manage configuration and credentials between infrastructure and services, all within the app model
- Simplify deployment with Bicep and Azure Resource Manager (ARM)

By the end of this tutorial you will be able to deploy eShop on containers with just a single command:

```sh
$ rad deploy eshop.bicep
Deploying Application 'eshop' to environment 'Azure'...
```

{{< button text="Next: Model eShop infrastructure" page="2-infrastructure" >}}

