---
type: docs
title: "Radius environments concept"
linkTitle: "Environments"
description: "Information on Radius environments and how they host Radius applications"
weight: 30
---

{{% alert title="😱 Warning" color="warning" %}}
This page is still an early work in progress, and is subject to change based upon continued Radius development.

- For now we only support Azure as the environment
- For now we use AKS as the runtime/data-plane
- For now you will see Radius implementation details as resources in your subscription
{{% /alert %}}


Environments are the combination of two things:
- A **control-plane** which communicates with with the rad CLI
- A **runtime** to which applications are deployed

## Control plane

The Radius control plane accepts application specs and deploys them into the data plane. Each [environment type]({{< ref environments>}}) has a different implementation, but the end result is that the rad CLI can deploy applications into the environment using the `rad deploy` command. This ensures that Radius applications are portable across environments.

For example, in [Microsoft Azure]({{< ref azure-environments >}}) the Radius control plane is the combination of an Azure Resource Manager (ARM) custom provider and an Azure App Service that orchestrates the deployment of Radius applications and components.

## Runtime

The Radius runtime is where Radius applications are deployed. It contains the container runtimes, database accounts, and other infrastructure into which Radius components and managed resources are deployed.

For example, in [Microsoft Azure]({{< ref azure-environments >}}) the Radius runtime is a Resource Group containing an Azure Kubernetes Service (AKS) cluster for running container components and other resources deployed as part of a Radius application.

## Supported platforms

Visit the [environments]({{< ref environments >}}) page for more information on supported environments.

## Related links

- [rad CLI reference]({{< ref cli >}})