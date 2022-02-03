---
type: docs
title: "Local Radius environments"
linkTitle: "Local machine"
description: "Run Radius applications locally on your machine"
weight: 20
---

## Overview

With a Radius local environment you can run your applications on your machine without the need for an Azure subscription or remote Kubernetes cluster. This makes it easy to develop applications and try them out without waiting for a full deployment to the cloud.

## Features

### Managed cluster runtime

A Radius local environment automatically creates a local Kubernetes cluster for you on top of Docker, making it easy to get up and running with an application runtime.

Use [`rad env init dev`]({{< ref rad_env_init_dev>}}) to create an environment.

### Local container registry

One of the slowest parts of working with containers can be waiting for container images to upload and download to remote registries. A Radius local environment automatically creates and manages a local container registry, making it easy to quickly transfer images into your local environment runtime.

### Hot reload

Radius local environments support hot reload in your development. **Guide coming soon**

## Initialize a local environment

### Prerequisites

- [rad CLI]({{< ref rad-cli >}})
- [Docker Desktop](https://www.docker.com/products/docker-desktop)
- [k3d](https://k3d.io/)

### Create a local dev environment

You can easily get up and running with a local environment with the [`rad env init dev` command]({{< ref rad_env_init_dev >}}):

```sh
rad env init dev
```

This will initialize a local Kubernetes cluster within Docker, along with a local container registry, Radius control-plane, and supporting services.

### Run applications in the local environment

Once you have a local environment, you can run Radius applications in it with the [`rad app run` command]({{< ref rad_application_run >}}):

```sh
rad app run
```

## Learn more

- [rad CLI reference]({{< ref cli >}})
