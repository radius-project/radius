---
type: docs
title: "Deploy eShop application to Radius environment"
linkTitle: "Deploy application"
slug: "deploy-app"
description: "Learn how to deploy thr eShop application to a Radius environment"
weight: 400
---

## Download complete Application

Make sure you have downloaded the latest eShop application:

{{< tabs Azure Kubernetes >}}
{{% codetab %}}
This template uses Azure SQL, Azure Redis Cache, Azure Service Bus, and Azure CosmosDB w/ Mongo API:
{{< rad file="../eshop-azure.bicep" download=true >}}
{{% /codetab %}}
{{% codetab %}}
This template uses containerized versions of SQL, Redis, RabbitMQ, and MongoDB:
{{< rad file="../eshop-kubernetes.bicep" download=true >}}
{{% /codetab %}}
{{% /tabs %}}

## Initialize environment

Visit the [getting started guide]({{< ref create-environment >}}) to deploy or connect to a Radius environment running the latest release.

### Get cluster IP

Radius gateways are still in development, and will get more features in upcoming releases. Until they are updated, manually retrieve your cluster IP address to pass into the application:

{{< tabs Azure Kubernetes >}}
{{% codetab %}}

1. Navigate to the RE-[ENV-NAME] resource group that was initialized for your environment.
1. Select the Azure Kubernetes Service cluster.
1. Note the IP address of the External IP of your LoadBalancer.
{{% /codetab %}}
{{% codetab %}}
1. Ensure your cluster is set as the default cluster in your kubectl config, and Radius is initialized on it.
1. Run `kubectl get svc -A` and note the EXTERNAL-IP value of your load balancer.
{{% /codetab %}}
{{% /tabs %}}

## Deploy application

Using the [`rad deploy`]({{< ref rad_deploy >}}) command, deploy the eShop application to your environment:

{{< tabs Azure Kubernetes >}}
{{% codetab %}}
```sh
$ rad deploy eshop-azure.bicep -p adminLogin=admin -p adminLoginPassword=YOUR-PASSWORD
```
{{% /codetab %}}
{{% codetab %}}
```sh
$ rad deploy eshop-kubernetes.bicep --parameters adminLogin=admin adminLoginPassword=YOUR-PASSWORD
```
{{% /codetab %}}
{{% /tabs %}}

{{% alert title="Note" color="info" %}}
Azure Redis cache can ~30 minutes to deploy. You can monitor your deployment process in the `Deployments` blade of your environment's resource group.
{{% /alert %}}

## Verify app health

Once deployed, verify eshop is up and running:

```sh
rad application show -a eshop
```

Then verify the health of the application resources:

```sh
rad resource list -a eshop
```

## Visit eShop

Now that eShop is deployed, you can visit the eShop application in your browser at the URL of your gateway:

```sh
rad resource show webmvc -a eshop
```

<img src="eshop.png" alt="Screenshot of the eShop application" width=1000 >