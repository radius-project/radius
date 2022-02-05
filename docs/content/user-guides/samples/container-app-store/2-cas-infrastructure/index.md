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

{{< button text="Download templates" link="https://radiuspublic.blob.core.windows.net/samples/container-app-microservices.zip" >}}

This directory contains the following files:

- **iac/app.bicep** - The application Bicep definition
- **iac/infra.azure.bicep** - The Dapr statestore definition for Azure
- **iac/infra.dev.bicep** - The Dapr statestore definition for local environments
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

{{< tabs "Azure Deployment" "Local Environment Deployment" >}}

{{< codetab >}}

{{< rad file="snippets/infra.azure.bicep" embed=true >}}
{{< /codetab >}}

{{< codetab >}}
{{< rad file="snippets/infra.dev.bicep" embed=true >}}
{{< /codetab >}}

{{< /tabs >}}

### Dev profile

Radius rad.yaml files allow users to use [profiles](http://localhost:1313/reference/rad-yaml/#profiles) which allow for specific customization in deciding which Bicep file properties are overwritten. For this example creating a `dev` profile and specifying that this profile will run the infra.dev.bicep file makes sure that when the `infra` stage is ran the current `statestore` is used.

```yaml
- name: infra
  bicep:
    template: iac/infra.bicep
  profiles:
    dev:
      bicep:
        template: iac/infra.dev.bicep
```

## Next steps

In the next step, you will learn about the Container App Store Microservice services.

{{< button text="Next: Model services" page="3-cas-services" >}}
