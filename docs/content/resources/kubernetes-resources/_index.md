---
type: docs
title: "Add Kubernetes resources to your Radius application"
linkTitle: "Kubernetes"
description: "Learn how to model and deploy Kubernetes resources as part of your application"
weight: 300
---

Radius applications are able to connect to and leverage Kubernetes resources with Bicep. Simply model your Kubernetes resources in Bicep and reference their properties in Radius.

You can import the Kubernetes types with:

```bicep
import kubernetes from kubernetes
```

## Resource library

Visit [GitHub](https://github.com/Azure/bicep-types-k8s/blob/main/generated/index.md) to reference the Kubernetes resource.

{{< button text="Kubernetes resource library" link="https://github.com/Azure/bicep-types-k8s/blob/main/generated/index.md" >}}

## Connections

Radius resources currently can reference Kubernetes resources directly without a connection. Connection support is coming soon.

### Example

{{< rad file="snippets/kubernetes-connection.bicep" embed=true >}}
