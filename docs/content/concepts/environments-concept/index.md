---
type: docs
title: "Radius environments concept"
linkTitle: "Environments"
description: "Information on Radius environments and how they host Radius applications"
weight: 30
---

{{% alert title="😱 Warning" color="warning" %}}
This page is still an early work in progress, and is subject to change based upon continued Radius development.
{{% /alert %}}


Environments, aka runtimes, are the combination of two things:
- A **control-plane** which communicates with with the rad CLI
- A **data-plane** to which applications are deployed

## Control plane

The Radius control plane accepts application specs and deploys them into the data plane. Each [environment type]({{< ref environments>}}) has a different implementation, but the end result is that the rad CLI can deploy applications into the environment using the `rad deploy` command. This ensures that Radius applications are portable across environments.

For example, in [Microsoft Azure]({{< ref azure-environments >}}) the Radius control plane is the combination of an Azure Resource Manager (ARM) custom provider and an Azure App Service that orchestrates the deployment of Radius applications and components.

## Data plane

The Radius data plane is where Radius applications are deployed. It contains the container runtimes, database accounts, and other infrastructure into which Radius components and managed resources are deployed.

For example, in [Microsoft Azure]({{< ref azure-environments >}}) the Radius data plane is a managed Resource Group containing an Azure Kubernetes Service (AKS) cluster for running container components, an Azure CosmosDB for mananged databases, and other resources deployed as part of a Radius application.

## Supported platforms

Visit the [environments]({{< ref environments >}}) page for more information on supported environments.

## Related links

- [rad CLI reference]({{< ref cli >}})