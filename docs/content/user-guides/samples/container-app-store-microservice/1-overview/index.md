---
type: docs
title: "Container App Store Microservice overview"
linkTitle: "Overview"
slug: "overview"
description: "Learn about the Container App Store Microservice sample application"
weight: 100
---

## Background

The [Container App Store Microservice](https://github.com/Azure-Samples/container-apps-store-api-microservice) reference application is a sample application that contains microservices written in Python, Go, NodeJS and uses Dapr to communicate between services.

### Architecture

<img src="architecture.png" alt="Architecture diagram of Container App Store Microservice" width=900 >

From the [Container App Store Microservice repo](https://github.com/Azure-Samples/container-apps-store-api-microservice/blob/main/assets/arch.png)):

> This is a sample microservice solution for Azure Container Apps. It will create a store microservice which will need to call into an order service and an inventory service. Dapr is used to secure communication and calls between services, and Azure API Management and Azure Cosmos DB are created alongside the microservices.

> There are three main microservices in the solution.
> Store API (node-app)
> The node-app is an express.js API that exposes three endpoints. / will return the primary index > page, /order will return details on an order (retrieved from the order service), and /inventory > will return details on an inventory item (retrieved from the inventory service).

> Order Service (python-app)
> The python-app is a Python flask app that will retrieve and store the state of orders. It uses > Dapr state management to store the state of the orders. When deployed in Container Apps, Dapr is configured to point to an Azure Cosmos DB to back the state.

> Inventory Service (go-app)
> The go-app is a Go mux app that will retrieve and store the state of inventory. For this sample, the mux app just returns back a static value.

### Deployment without Radius

[Deploying Container App Store Microservice without Radius](https://github.com/Azure-Samples/container-apps-store-api-microservice#deploy) contains two options, deploying through GitHub Actions or Azure Bicep.
> The entire solution is configured with GitHub Actions and Bicep for CI/CD.


Container App Store Microservice provides instructions to [deploy to Azure via Bicep](https://github.com/Azure-Samples/container-apps-store-api-microservice#deploy-via-azure-bicep) or to [deploy via GitHub Actions](https://github.com/Azure-Samples/container-apps-store-api-microservice#deploy-via-github-actions-recommended)

## Adding Radius

Adding Project Radius to the Container App Store Microservice on containers application allows teams to:

- Define the entire collection of microservices and backing infrastructure as a single application
- Easily manage configuration and credentials between infrastructure and services, all within the app model
- Simplify deployment with Bicep and Azure Resource Manager (ARM)

By the end of this sample you will be able to deploy Container App Store Microservice both locally with Radius or to the cloud using Radius.


{{< button text="Next: Model Container App Store Microservice infrastructure" page="2-infrastructure" >}}
