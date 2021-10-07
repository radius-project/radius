---
type: docs
title: "Model eShop services in Radius"
linkTitle: "Model services"
slug: "model-services"
description: "Learn how to model the eShop services in Radius"
weight: 300
---

## Add parameters

To start, add the following parameters to the top of your `eshop.bicep` file:

{{< rad file="snippets/eshop.bicep" embed=true marker="//PARAMS" >}}

## Add catalog service

Within your `eshop` resource, add the following [ContainerComponent]({{< ref container >}}) resource for the catalog service:

{{< rad file="snippets/eshop.bicep" embed=true marker="//CATALOG" replace-key-provides="//PROVIDES" replace-value-provides="" >}}

Note the various pieces of the container definition.

### Image

The catalog service uses the `'eshop/catalog.api:latest'` image.

### Environment variables

Within the `env` section of the container definition, note the different types of values:

- Static values set within the container definition (*eg. `'PATH_BASE': '/catalog-api'`*)
- Global parameters defined in the `eshop.bicep` file, that can also be passed in at deloy time (*eg. `'OrchestratorType': OCHESTRATOR_TYPE`*)
- Resource values accessed as references to other Radius resources (*eg. `'ConnectionString': sqlCatalog.connectionString()`*)

### Ports

You can define the ports that the catalog service will offer within `ports`. We'll revisit these later.

### Connections

The catalog service uses the `sqlCatalog` and `servicebus` resources. Within `connections` these relationships are defined, ensuring the proper RBAC assignments are set. Visualization and management experiences use this information later on.

## Add an HTTP Route

Other services will communicate with the catalog service via HTTP. The catalog endpoint also needs to be accessible publicly for users to interact with. Add the following [HttpRouteComponent]({{< ref http-route >}}) resource to the `eshop` resource for other services to connect to, with a `gateway` defined to create the public endpoint:

{{< rad file="snippets/eshop.bicep" embed=true marker="//ROUTE" replace-key-provides="//PROVIDES" replace-value-provides="provides: catalogHttp.id" >}}

Update your catalog `ports` definition so the `http` port provides `catalogHTTP`:

```sh
http: {
  containerPort: 80
  provides: catalogHttp.id
}
```

## Add remaining services

Now that you've defined the catalog service, you can add the remaining services. Downlaod the full `eshop.bicep` template to see all the eShop services:

{{< rad file="snippets/eshop.bicep" download=true >}}

## Next steps

Now that you have the eShop infrastructure and services modeled, you can deploy eShop to a Radius environment.

{{< button text="Next: Deploy eShop application" page="4-deploy" >}}
