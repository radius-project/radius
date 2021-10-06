---
type: docs
title: "Radius container component"
linkTitle: "Container"
description: "Learn about the Radius container component"
weight: 100
---

`ContainerComponent` provides an abstraction for a container workload that can be run on any [Radius platform]({{< ref platforms >}}).

## Platform resources

Containers are hosted by the following runtimes on each platform:

| Platform | Resource |
|----------|----------|
| [Microsoft Azure]({{< ref azure>}}) | Kubernetes Deployment on AKS |
| [Kubernetes]({{< ref kubernetes >}}) | Kubernetes Deployment |

## Component format

{{< rad file="snippets/container.bicep" embed=true marker="//CONTAINER" >}}

The following top-level information is available for containers:

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | The name of your component. Used to provide status and visualize the component. | `frontend`

### Container

Deatils on what to run and how to run it are defined in the `container` property:

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| image | y | The registry and image to download and run in your container. | `radiusteam/frontend`
| env | n | The environment variables to be set for the container. | `"ENV_VAR": "value"`
| ports | n | Ports the container provides | [See below](#ports).

### Ports

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | A name key for the port. | `http`
| containerPort | y | The port the container exposes | `80`
| protocol | n | The protocol the container exposes | `'TCP'`
| provides | n | The id of the [Route]({{< ref connections-model >}}) the container provides. | `http.id`

### Volumes

{{% alert title="🚧 Under construction" color="warning" %}}
Volumes are currently being developed. Stay tuned for an update.
{{% /alert %}}

### Connections

Connections define how a container connects to [other resources]({{< ref resources >}}).

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | A name key for the port. | `inventory`
| kind | y | The type of resource you are connecting to. | `mongo.com/MongoDB`
| source | y | The id of the [Component]({{< ref components-model >}}) the container is connecting to. | `db.id`
