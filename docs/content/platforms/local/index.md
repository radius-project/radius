---
type: docs
title: "Local Radius environments"
linkTitle: "Local machine"
description: "Run Radius applications locally on your machine"
weight: 20
---

## Overview

With a Radius local environment you can run your applications on your machine without the need for an Azure subscription or remote Kubernetes cluster. This makes it easy to develop applications and try them out without waiting for a full deployment.

Try one out as part of the [Container App Store sample]({{< ref container-app-store-microservice >}}).

## Features

### Manage Kubernetes clusters

Radius creates a local cluster for you automatically once a local environment has been created allowing you to not worry about understanding how to setup Docker and Kubernetes together. As well as providing additional Kubernetes specific actions in the `rad cli` such as:

- Merge Kubernetes credentials (`rad env merge-credentials <target>`)

As well as giving users the flexibility to handle any customizations for Kubernetes on their own after the initial Kubernetes cluster is created by Radius.

## Managing environments

Radius envinroments can be managed through the `rad cli` in various commands such as:

- Show RAD environment details (`rad env show <target>`)
- Show local Radius environment status (`rad env status <target>`)
- Start a local Radius environment (`rad env start <target>`)
- Stop a local Radius environment (`rad env stop <target>`)
- Switch the current environment (`rad env switch <target>`)
- Delete environment (`rad env delete <target> `)

### Local container registry

Any information in regards to your containers can be easily accessed either through `rad cli` commands stated above if they relate to the Radius environment or through normal Docker and Kubernetes commands.

### Hot reload

Radius local environments support hot reload in your development with technologies such as nodemon.

### Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop)
- [rad CLI] ({{< ref install-cli.md >}})
- [Kubernetes](https://kubernetes.io/)

### Create a local dev environment

You can easily get up and running with a local environment with the command:

```sh
rad env init dev
```

This will initialize a local Kubernetes cluster within Docker, along with a local container registry, Radius control-plane, and supporting services.


### Run applications in the local environment

Once you have a local environment, you can run Radius applications in it with the command:

```sh
rad app run ....
```

## Example

Check out the [Container Apps Store sample]({{< ref container-app-store-microservice.md>}}) to try out a local dev experience.

## Learn More

- [rad cli documentation]({{< ref rad.md >}})
