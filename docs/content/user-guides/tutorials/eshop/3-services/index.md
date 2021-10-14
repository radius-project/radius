---
type: docs
title: "Model eShop services in Radius"
linkTitle: "Model services"
slug: "model-services"
description: "Learn how to model the eShop services in Radius"
weight: 300
---

## Add parameters

Add the following parameters to the top of your `eshop.bicep` file along with the other parameters you've specified, which will be used by eShop.

{{< rad file="snippets/catalog.bicep" embed=true marker="//PARAMS" replace-key-rest="//REST" replace-value-rest="..." >}}

## Add catalog service

Within your `eshop` resource, add the following [ContainerComponent]({{< ref container >}}) resource for the catalog service. Note the use of other Radius Components for connection information.

{{< rad file="snippets/catalog.bicep" embed=true marker="//CATALOG" replace-key-provides="//PROVIDES" replace-value-provides="" >}}

### Image

The catalog service uses the `'eshop/catalog.api:${TAG}'` image.

### Environment variables

Within the `env` section of the container definition, note the different types of values:

- Static values set within the container definition (*eg. `'PATH_BASE': '/catalog-api'`*)
- Global parameters defined in the `eshop.bicep` file, that can also be passed in at deloy time (*eg. `'OrchestratorType': OCHESTRATOR_TYPE`*)
- Resource values accessed as references to other Radius resources (*eg. `'ConnectionString': sqlCatalog.properties.server`*)
- Resource values from non-Radius resources (*eg. `listKeys(servicebus::topic::rootRule.id, servicebus::topic::rootRule.apiVersion).primaryKey`*)

### Ports

The Catalog api service offers two ports: http and grpc. Other services can access these ports though *Routes*, which we'll cover soon.

### Connections

The Catalog service can connect to other Radius resources via the `connections` section. For Azure this will be SQL (platform-specific resources like Service Bus don't yet support connections). For Kubernetes this will be RabbitMQ and the sqlRoute.

## Add an HTTP Route

Other services will communicate with the catalog service via HTTP. The catalog endpoint also needs to be accessible publicly for users to interact with. Add the following [HttpRouteComponent]({{< ref http-route >}}) resource to the `eshop` resource for other services to connect to, with a `gateway` defined to create the public endpoint:

{{< rad file="snippets/catalog.bicep" embed=true marker="//ROUTE" replace-key-provides="//PROVIDES" replace-value-provides="provides: catalogHttp.id" >}}

Update your catalog `ports` definition so the `http` port provides `catalogHTTP`:

```sh
http: {
  containerPort: 80
  provides: catalogHttp.id
}
```

## Add remaining services

Now that you've defined the catalog service, you can add the remaining services. Download the full `eshop.bicep` template to see all the eShop services:

{{< tabs Azure Kubernetes >}}

{{% codetab %}}
{{< rad file="../eshop-azure.bicep" download=true >}}
{{% /codetab %}}

{{% codetab %}}
{{< rad file="../eshop-kubernetes.bicep" download=true >}}
{{% /codetab %}}

{{< /tabs >}}

## Next steps

Now that you have the eShop infrastructure and services modeled, you can deploy eShop to a Radius environment.

{{< button text="Next: Deploy eShop application" page="4-deploy" >}}
