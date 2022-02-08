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

{{< button text="Download templates" link="https://get.radapp.dev/samples/container-app-microservices.zip" >}}

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

## Dapr state store connector

A [Dapr state store connector]({{< ref dapr-statestore >}}) resource is required by the container apps store microservices.

Within `infra.dev.bicep` and `infra.bicep` you will find the following resources:

{{< tabs "infra.dev.bicep" "infra.bicep" >}}

{{< codetab >}}
{{< rad file="snippets/infra.dev.bicep" embed=true >}}
{{< /codetab >}}

{{< codetab >}}
{{< rad file="snippets/infra.bicep" embed=true >}}
{{< /codetab >}}

{{< /tabs >}}

`infra.dev.bicep` is used for development environments and `infra.bicep` is used for production environments.

## rad.yaml stages and profiles

A [rad.yaml]({{< ref rad-yaml >}}) file allow users to define stages and profiles of deployment.

This sample contains `infra` and `app` stages, along with a `dev` profile. The `dev` profile tells Radius to substitute `infra.bicep` for `infra.dev.bicep`:

```yaml
name: store
stages:
- name: infra
  bicep:
    template: iac/infra.bicep
  profiles:
    dev:
      bicep:
        template: iac/infra.dev.bicep
...
```

## Next steps

In the next step, you will learn about the Container App Store Microservice services.

{{< button text="Next: Model services" page="3-cas-services" >}}
