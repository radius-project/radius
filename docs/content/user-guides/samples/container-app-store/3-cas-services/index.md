---
type: docs
title: "Model Container App Store Microservice services"
linkTitle: "Services"
slug: "services"
description: "Learn how to model the Container App Store Microservice services"
weight: 300
---

## Building the services

As part of a Radius deployment, the container images are built and provided to the application as parameters.

Open [`rad.yaml`]({{< ref rad-yaml >}}) to see where the containers are built:

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
- name: app
  build:
    go_service_build:
      docker:
        context: go-service
        image: MYREGISTRY/go-service
    node_service_build:
      docker:
        context: node-service
        image: MYREGISTRY/node-service
    python_service_build:
      docker:
        context: python-service
        image: MYREGISTRY/python
  ...
```

Within `app.bicep`, object parameters with the same name as the build steps are available to use with the container image imformation from the above step:

{{< rad file="snippets/app.bicep" embed=true marker="//PARAMS" >}}

## Services

The Container App Store services are modeled as Radius [container resources]({{< ref container >}}):

{{< tabs "Store API (node-app)" "Order Service (python-app)" "Inventory Service (go-app)" >}}

{{% codetab %}}
{{< rad file="snippets/app.bicep" embed=true marker="//NODEAPP" replace-key-rest="//REST" replace-value-rest="..." >}}
{{% /codetab %}}

{{% codetab %}}
{{< rad file="snippets/app.bicep" embed=true marker="//PYTHONAPP" replace-key-rest="//REST" replace-value-rest="..." >}}
{{% /codetab %}}

{{% codetab %}}
{{< rad file="snippets/app.bicep" embed=true marker="//GOAPP" replace-key-rest="//REST" replace-value-rest="..." >}}
{{% /codetab %}}

{{< /tabs >}}

Note the [`dapr.io/Sidecar` trait]({{< ref dapr-trait >}}) to add Dapr to each service.

## HTTP Routes

Each service will communicate with each other via HTTP.

An [Http Route]({{< ref http-route >}}) resource allows services to communicate with eachother. Gateways can also be added to expose the service over the internet.

{{< rad file="snippets/app.bicep" embed=true marker="//ROUTE">}}

## Dapr HTTP Routes

The Python service allows other services to invoke it using Dapr Service Invocation, using a [Dapr Invoke Route]({{< ref dapr-http >}}):

{{< rad file="snippets/app.bicep" embed=true marker="//DAPR" >}}

## Next steps

Now that we have modeled the infrastructure and services let's run it locally on a Radius dev environment.

{{< button text="Next: Run application locally" page="4-cas-run" >}}
