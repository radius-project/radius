---
type: docs
title: "Add Kubernetes resources to your Radius application"
linkTitle: "Kubernetes"
description: "Learn how to model and deploy Kubernetes resources as part of your application"
weight: 300
---

Kubernetes types can be modeled in Bicep through the `kubernetes` extension.

## Resource library

{{% alert title="Coming soon" color="info" %}}
Kubernetes resource docs are coming soon. In the meantime you can use the [VS Code tooling]({{< ref setup-vscode >}}) to discover and learn about Kubernetes resource types.
{{% /alert %}}

## Kubernetes connections

Radius resources currently can reference Kubernetes resources directly without a connection. Connection support is coming soon.

{{< rad file="snippets/kubernetes-connection.bicep" embed=true >}}
