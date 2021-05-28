---
type: docs
title: "Radius container component"
linkTitle: "Container"
description: "Learn about the Radius container component"
weight: 1000
---

The `radius.dev/Container` component provides an abstraction for a container workload that can be run on any [Radius platform]({{< ref environments >}}).

## Configuraiton

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of your component. Used to provide status and visualize the component. | `frontend`
| kind | y |The component kind and version. | `radius.dev/Container@v1alpha1`
| properties.run.container.image | y | The registry and image to download and run in your container. | `radiusteam/frontend`
| properties.provides |  | Services that your container provides to other components. See [services](#services) for more  information | -

### Environment variables

Environment variables can be configured automatically via the component [`dependsOn`configuration]({{< ref "components-model.md#dependson" >}}).

## Services

### HTTP endpoint

The `http` service provides an HTTP endpoint service which opens a specified port on a container so that other services can connect to endpoints listening on the container.

| Key | Required | Description | Example |
|-----|:--------:|-------------|---------|
| kind | y | Defines the service type. | `http`
| name | Y | The name used to describe the component. Used when providing status and visualizing your application & component. | `webserver`
| containerPort | Y | The HTTP port to open on the container for other components to access. | `443`

```sh
resource frontend 'Components' = {
  name: 'frontend'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {...}
    provides: [
      {
        name: 'frontend'
        kind: 'http'
        containerPort: 80
      }
    ]
  }
}
```

## Traits

### Inbound route

The `radius.dev/InboundRoute` trait adds an ingress controller to the container component to accept HTTP traffic from the internet.

| Key | Required | Description | Example |
|-----|:--------:|-------------|---------|
| kind | y | Defines the trait type. | `'radius.dev/InboundRoute@v1alpha1'`
| properties.service | y | The service to create an ingress controller on and expose to the internet. | `'frontend'`

{{< rad file="ingress.bicep" >}}

```sh
resource frontend 'Components' = {
  name: 'frontend'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {...}
    provides: [
      {
        name: 'frontend'
        kind: 'http'
        containerPort: 80
      }
    ]
    traits: [
      {
        kind: 'radius.dev/InboundRoute@v1alpha1'
        properties: {
          service: 'frontend'
        }
      }
    ]
  }
}
```

### Dapr sidecar

The `dapr.io/App` trait adds a [Dapr](https://dapr.io) sidecar to the container, and ensures a Dapr control plane is deployed to the underlying hosting platform. This allows you to use all of the Dapr building blocks and APIs from your container.

| Key | Required | Description | Example |
|-----|:--------:|-------------|---------|
| appId | y | The unique name for  | `http`
| appPort | y | The name used to describe the component. Used when providing status and visualizing your application & component. | `webserver`

```sh
resource frontend 'Components' = {
  name: 'frontend'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {...}
    traits: [
      {
        kind: 'dapr.io/App@v1alpha1'
        properties: {
          appId: 'frontend'
          appPort: 3000
        }
      }
    ]
  }
}
```

## Example

The following example shows a container component that provides an HTTP service on port 3000 and has a Dapr app trait.

```sh
resource todoapplication 'Components' = {
  name: 'todoapp'
  kind: 'radius.dev/Container@v1alpha1'
  properties: {
    run: {
      container: {
        image: 'radiusteam/tutorial-todoapp'
      }
    }
    provides: [
      {
        kind: 'http'
        name: 'web'
        containerPort: 3000
      }
    ]
    traits: [
      {
        kind: 'dapr.io/App@v1alpha1'
        properties: {
          appId: 'todoapp'
          appPort: 3000
        }
      }
  }
}
```