---
type: docs
title: "Use Dapr with Radius"
linkTitle: "Dapr sidecar"
description: "Learn how to use Dapr with Radius"
weight: 100
---

## Background

Without Radius, there are multiple steps to add Dapr to a containerized application:

1. Download the Dapr CLI
1. Initialize Dapr on your Kubernetes cluster
1. Add Dapr annotations to your container detailing app-id and app-port

## Dapr App component

The Radius Dapr app component offers to the user:

- Automatic Dapr control plane management
- Automatic sidecar configuration and injection

### Add a Dapr sidecar to a container

To add a Dapr sidecar to a container simply add a `dapr.io/App` trait to a container:

```sh
resource nodeapp 'Components' = {
  name: 'nodeapp'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {...}
  traits: [
    {
      kind: 'dapr.io/App@v1alpha1'
      properties: {
        appId: 'nodeapp'
        appPort: 3000
      }
    }
  ]
}
```

## Tutorial

Based on: https://github.com/dapr/samples/tree/master/hello-docker-compose

### Bicep file

{{< rad file="template.bicep">}}