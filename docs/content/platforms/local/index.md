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

### Managed cluster runtime

A Radius local environment automatically creates a local Kubernetes cluster for you on top of Docker, making it easy to get up and running with an application runtime.

Use [`rad env init dev`]({{< ref rad_env_init_dev>}}) to create an environment.

### Local container registry

One of the slowest parts of working with containers can be waiting for container images to upload and download to remote registries.  Radius local environments automatically creates and manages a local container registry, making it easy to quickly push and pull images.

### Hot reload

Radius local environments support hot reload in your development with technologies such as nodemon/NodeJs and Docker. The same type of configurations that can be made in your Docker files for supporting hot reload should be compatible. An example of a Docker file configuration for hot reloading would be this:

```dockerfile
FROM node:14-alpine

USER node
RUN mkdir -p /home/node/app
WORKDIR /home/node/app

COPY --chown=node:node package*.json ./
RUN npm ci
COPY --chown=node:node . .

EXPOSE 3000
ARG ENV=development
ENV NODE_ENV $ENV
CMD ["npm", "run", "watch"]
```

In addition don't forget that for hot reloading you need to change your package.json file to support `watch` such as this:

```json
{
  "name": "node-service",
  "version": "0.0.0",
  "private": true,
  "scripts": {
    "start": "node ./bin/www",
    "watch": "nodemon ./bin/www"
  },
  "dependencies": {
    "axios": "^0.22.0",
    "cookie-parser": "~1.4.4",
    "debug": "~2.6.9",
    "express": "~4.16.1",
    "http-errors": "~1.6.3",
    "jade": "~1.11.0",
    "morgan": "~1.9.1"
  },
  "devDependencies": {
    "nodemon": "^2.0.15"
  }
}

```

## Initialize a local environment

### Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop)
- [rad CLI] ({{< ref install-cli.md >}})

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

## Example

Check out the [Container Apps Store sample]({{< ref container-app-store-microservice.md>}}) to try out a local dev experience.

## Learn more

- [rad CLI reference]({{< ref cli >}})
