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
| properties.bindings |  | Bindings that your container provides to other components. See [bindings](#bindings) for more  information | -

## Bindings

### HTTP endpoint

The `http` binding provides an HTTP endpoint which opens a specified port on a container so that other components can send HTTP traffic to the container.

| Key | Required | Description | Example |
|-----|:--------:|-------------|---------|
| kind | y | Defines the binding type. | `http`
| targetPort | y | The HTTP port your application is listening on inside the container. Defaults to value of `port`. | `3000`
| port | n | The port to serve the HTTP binding on. Defaults to `80`. | `3500`

{{< rad file="snippets/frontend-backend.bicep" embed=true marker="//SAMPLE" replace-key-run="//HIDE" replace-value-run="run: {...}" >}}

## Dapr invoke

The `dapr.io/Invoke` binding indicates that other components can invoke a service on this container using [Dapr service invocation](https://docs.dapr.io/developing-applications/building-blocks/service-invocation/).

| Key | Required | Description | Example |
|-----|:--------:|-------------|---------|
| kind | y | Defines the binding type. | `dapr.io/Invoke`

## Traits

### Inbound route

The `radius.dev/InboundRoute` trait adds an ingress controller to the container component to accept HTTP traffic from the internet.

| Key | Required | Description | Example |
|-----|:--------:|-------------|---------|
| kind | y | Defines the trait type. | `'radius.dev/InboundRoute@v1alpha1'`
| binding | y | The binding to create an ingress controller on and expose to the internet. | `'frontend'`
| hostname | n | The hostname to use for the inbound route. | `example.com`

{{< rad file="snippets/inboundroute.bicep" embed=true marker="//SAMPLE" replace-key-run="//HIDE" replace-value-run="run: {...}" >}}

### Dapr sidecar

The `dapr.io/App` trait adds a [Dapr](https://dapr.io) sidecar to the container, and ensures a Dapr control plane is deployed to the underlying hosting platform. This allows you to use all of the Dapr building blocks and APIs from your container.

| Key | Required | Description | Example |
|-----|:--------:|-------------|---------|
| kind | y | Defines the trait type. | `'dapr.io/App@v1alpha1'`
| appId | y | The unique name for your Dapr application. | `frontend`
| appPort | y | The port that Dapr proxy will use to expose the service to clients | `3000`

{{< rad file="snippets/dapr.bicep" embed=true marker="//SAMPLE" replace-key-run="//HIDE" replace-value-run="run: {...}" >}}
