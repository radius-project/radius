---
type: docs
title: "Model eShop services in Radius"
linkTitle: "Services"
slug: "model-services"
description: "Learn how to model the eShop services in Radius"
weight: 300
---

## Add parameters

The following parameters are added to the eShop file and referenced by multiple services:

{{< rad file="snippets/catalog.bicep" embed=true marker="//PARAMS" replace-key-rest="//REST" replace-value-rest="..." >}}

{{% alert title="Note" color="info" %}}
Improved gateway DNS support is in development. In the meantime, [nip.io](https://nip.io) is recommended for DNS resolution to your Cluster IP address.
{{% /alert %}}

## Catalog service

Taking a closer look at a service, the catalog microservice is modeled as a [ContainerComponent]({{< ref container >}}) resource:

{{< rad file="snippets/catalog.bicep" embed=true marker="//CATALOG" replace-key-provides="//PROVIDES" replace-value-provides="" >}}

{{% alert title="⚠️ Connections to non-Radius resoures" color="info" %}}
While connections to non-Radius Bicep resources are not supported today, we are actively working on supporting this feature. In the meantime you can still access parameters and keys/passwords via the [list* functions](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/bicep-functions-resource#list).
{{% /alert %}}

### Image

The catalog service uses the `'eshop/catalog.api:${TAG}'` image.

### Environment variables

Within the `env` section of the container definition, note the different types of values:

- Static values set within the container definition (*eg. `'PATH_BASE': '/catalog-api'`*)
- Global parameters defined in the `eshop.bicep` file, that can also be passed in at deloy time (*eg. `'OrchestratorType': OCHESTRATOR_TYPE`*)
- Resource values accessed as references to other Radius resources (*eg. `'ConnectionString': sqlCatalog.properties.server`*)
- Resource values from non-Radius resources (*eg. `listKeys(servicebus::topic::rootRule.id, servicebus::topic::rootRule.apiVersion).primaryKey`*)

### Ports

The Catalog API service offers two ports: http and grpc. Other services can access these ports though *Routes*, which we'll cover soon.

### Connections

The Catalog service can connect to other Radius resources via the `connections` section. For Azure this will be SQL (platform-specific resources like Service Bus don't yet support connections). For Kubernetes this will be RabbitMQ and the sqlRoute.

## HTTP Route

Other services will communicate with the catalog service via HTTP.

An [HttpRouteComponent]({{< ref http-route >}}) resource allows other resources to connect to the `eshop` resource:

{{< rad file="snippets/catalog.bicep" embed=true marker="//ROUTE" replace-key-provides="//PROVIDES" replace-value-provides="provides: catalogHttp.id" >}}

Catalog's `ports` definition provides `catalogHTTP`:

```sh
http: {
  containerPort: 80
  provides: catalogHttp.id
}
```

While catalog is not exposed to the internet, you can optionally add a `gateway` property to the HttpRoute to expose it to the internet. Other services in eShop use gateways.

## Next steps

Now that we have looked at the eShop infrastructure, and how we can model its services, let's now deploy it to a Radius environment.

{{< button text="Next: Deploy eShop application" page="4-deploy" >}}
