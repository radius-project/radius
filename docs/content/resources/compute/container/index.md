---
type: docs
title: "Radius container component"
linkTitle: "Container"
description: "Learn about the Radius container component"
weight: 100
---

`Container` provides an abstraction for a container workload that can be run on any [Radius platform]({{< ref platforms >}}).

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

Details on what to run and how to run it are defined in the `container` property:

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| image | y | The registry and image to download and run in your container. | `radiusteam/frontend`
| env | n | The environment variables to be set for the container. | `"ENV_VAR": "value"`
| ports | n | Ports the container provides | [See below](#ports).
| readinessProbe | n | Readiness probe configuration. | [See below](#readiness-probe).
| livenessProbe | n | Liveness probe configuration. | [See below](#liveness-probe).

### Ports

The ports offered by the container are  defined in the `ports` section.

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | A name key for the port. | `http`
| containerPort | y | The port the container exposes | `80`
| protocol | n | The protocol the container exposes | `'TCP'`
| provides | n | The id of the [Route]({{< ref networking >}}) the container provides. | `http.id`

### Volumes

The volumes mounted to the container, either local or external, are defined in the `volumes` section.
Learn more about volumes and supported resources in the [Volume docs]({{< ref volume >}}).

### Readiness probe

Readiness probes detect when a container begins reporting it is ready to receive traffic, such as after loading a large configuration file that may take a couple seconds to process.
There are three types of probes available, `httpGet`, `tcp` and `exec`. For an `httpGet` probe, an HTTP GET request at the specified endpoint will be used to probe the application. If a success code is returned, the probe passes. If no code or an error code is returned, the probe fails, and the container won't receive any requests after a specified number of failures.
Any code greater than or equal to 200 and less than 400 indicates success. Any other code indicates failure.

For a `tcp` probe, the specified container port is probed to be listening. If not, the probe fails.

For an `exec` probe, a command is run within the container. A return code of 0 indicates a success and the probe succeeds. Any other return indicates a failure, and the container doesn't receive any requests after a specified number of failures.

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | Type of readiness check, `httpGet` or `tcp` or `exec`. | `httpGet`
| containerPort | n | Used when kind is `httpGet` or `tcp`. The listening port number. | `8080`
| path | n | Used when kind is `httpGet`. The route to make the HTTP request on | `'/healthz'`
| command | n | Used when kind is `exec`. Command to execute to probe readiness | `'/healthz'`
| initialDelaySeconds | n | Initial delay in seconds before probing for readiness. | `10`
| failureThreshold | n | Threshold number of times the probe fails after which a failure would be reported. | `5`
| periodSeconds | n | Interval for the readiness probe in seconds. | `5`

### Liveness probe

Liveness probes detect when a container is in a broken state, restarting the container to return it to a healthy state.
There are three types of probes available, `httpGet`, `tcp` and `exec`. For an `httpGet` probe, an HTTP GET request at the specified endpoint will be used to probe the application. If a success code is returned, the probe passes. If no code or an error code is returned, the probe fails, and the container won't receive any requests after a specified number of failures.
Any code greater than or equal to 200 and less than 400 indicates success. Any other code indicates failure.

For a `tcp` probe, the specified container port is probed to be listening. If not, the probe fails.

For an `exec` probe, a command is run within the container. A return code of 0 indicates a success and the probe succeeds. Any other return indicates a failure, and the container doesn't receive any requests after a specified number of failures.

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| kind | y | Type of liveness check, `httpGet` or `tcp` or `exec`. | `httpGet`
| containerPort | n | Used when kind is `httpGet` or `tcp`. The listening port number. | `8080`
| path | n | Used when kind is `httpGet`. The route to make the HTTP request on | `'/healthz'`
| command | n | Used when kind is `exec`. Command to execute to probe liveness | `'/healthz'`
| initialDelaySeconds | n | Initial delay in seconds before probing for liveness. | `10`
| failureThreshold | n | Threshold number of times the probe fails after which a failure would be reported. | `5`
| periodSeconds | n | Interval for the liveness probe in seconds. | `5`

### Connections

Connections define how a container connects to [other resources]({{< ref resources >}}).

| Key  | Required | Description | Example |
|------|:--------:|-------------|---------|
| name | y | A name key for the port. | `inventory`
| kind | y | The type of resource you are connecting to. | `mongo.com/MongoDB`
| source | y | The id of the [Component]({{< ref components-model >}}) the container is connecting to. | `db.id`
