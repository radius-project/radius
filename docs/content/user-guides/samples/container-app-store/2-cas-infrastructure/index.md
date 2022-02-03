---
type: docs
title: "Add infrastructure to Container App Store sample application"
linkTitle: "Infrastructure"
slug: "infrastructure"
description: "Learn how to model the Container App Store Microservice infrastructure in Bicep"
weight: 200
---

In this section you will be defining the Container Apps Store application and infrastructure that compose it.

## Download the sample application

Begin by downloading the sample templates from the following link:

{{< button text="Download templates" link="https://get.radapp.dev" >}}

This directory contains the following files:

- **iac/app.bicep** - The application Bicep definition
- **rad.yaml** - The [application configuration]({{< ref rad-yaml >}})
- **go-service/** - The go microservice source code
- **node-service/** - The node microservice source code
- **python-service/** - The python microservice source code

## Radius application

Note the [Radius Application resource]({{< ref application-model >}}) inside of `infra.bicep`:

{{< rad file="snippets/blank-app.bicep" embed=true >}}

## Infrastructure

The following resources are required by the container apps store microservices:

- Dapr state store

Add the following [connector]({{< ref connectors >}}) resources inside your application:

{{< rad file="snippets/app.bicep" embed=true marker="//RESOURCES" >}}

### Dev profile

//TODO

## Next steps

In the next step, you will learn about the Container App Store Microservice services.

{{< button text="Next: Model services" page="3-cas-services" >}}
