---
type: docs
title: "Running Radius resource provider locally"
linkTitle: "Run RP locally"
description: "How to get a local deployment of the Radius resource provider up and running"
weight: 20
---

You can run the Radius RP locally using:

- MongoDB in a container
- Your local Kubernetes credentials
- Your local Azure credentials

This will enable to use an ephemeral database as well as your local changes to the RP (built from source).

## Step 1: Running MongoDB

The resource provider uses MongoDB (CosmosDB in production). For local testing we can provide this via Docker. However, Mongo does some complicated networking things that require a complex setup. The instructions here will run MongoDB in a compatible way with the connection string provided above.

> :bulb: When you follow these instructions to run MongoDB the data is stored in memory in the container and will not persist across restarts. This can be beneficial because you always start with clean slate.

> :warning: Since the data is stored in memory, no cleanup operations happen when you shut down the database/RP. You might leave resources-sitting around in Azure or Kubernetes as a result.

**Adding an /etc/hosts Entry**

Add an entry to `/etc/hosts` to map `mongo` to `127.0.0.1`.

A non-localhost hostname is required if you're running the resource provider in your host environment. You can choose a different hostname if you want, but update both the `docker run` command below and the connection string above to match if you do so.

**Launching MongoDB**

```sh
# Run mongo in a container
# - using the standard mongo port (27017)
# - with 'mongoadmin' as a username
# - with 'secret' as the password
# - with 'rpdb' as the database name
docker run -d \
    -p 27017:27017 \
    --hostname mongo \
    -e MONGO_INITDB_ROOT_USERNAME=mongoadmin \
    -e MONGO_INITDB_ROOT_PASSWORD=secret \
    -e MONGO_INITDB_DATABASE=rpdb \
    mongo
```

If you need to connect to this using the MongoDB CLI then you can do so like:

```sh
mongo -u mongoadmin -p secret -authenticationDatabase admin rpdb
```

## Configuring the RP

**TLDR:**

```sh
export SKIP_AUTH='true'
export PORT='5000'
export MONGODB_CONNECTION_STRING='mongodb://mongoadmin:secret@mongo:27017/rpdb?authSource=admin'
export MONGODB_DATABASE='rpdb'
export K8S_LOCAL=true
export ARM_RESOURCE_GROUP="$(whoami)-radius"
export ARM_SUBSCRIPTION_ID="$(az account show --query 'id'  --output tsv)"
```

Configures all of the required environment variables:

- Listening on port 5000
- Using a local MongoDB container
- Using your local Kubernetes configuration
- Using your local `az` CLI config to talk to a specified resource group

**Explanation:**

The RP requires several environment variables to run (required configuration values).

We require specifying the port via the `PORT` environment variable.

You need to bypass certificate validation with `SKIP_AUTH=true`

We require configuration for connecting to MongoDB:

- `MONGODB_CONNECTION_STRING`
- `MONGODB_DATABASE`

We require configuration for connecting to a Kubernetes cluster. You have some options...

- Use local Kubernetes configuration:
  - `K8S_LOCAL=true`
- Retrieve a Kubernetes configuration from ARM using the az CLI to authorize:
  - `K8S_CLUSTER_NAME`
  - `K8S_RESOURCE_GROUP`
  - `K8S_SUBSCRIPTION_ID`
- Retrieve a Kubernetes configuration from ARM using a service principal to authorize (production scenario):
  - `K8S_CLUSTER_NAME`
  - `K8S_RESOURCE_GROUP`
  - `K8S_SUBSCRIPTION_ID`
  - `CLIENT_ID`
  - `CLIENT_SECRET`
  - `TENANT_ID`

We optionally require configuration for managing Azure resources:

- We first look for:
  - `ARM_RESOURCE_GROUP`
  - `ARM_SUBSCRIPTION_ID`

The simplest is to use your local configuration for Kubernetes (assuming it's already set up) and some defaults for Azure

## Step 3: Running the RP

Use `go run` to launch the RP from the same terminal where you configured the environment variables.

```sh
go run cmd/rp/main.go
```

## Optional: Debugging the RP

Launch VSCode from the same terminal where you configurred the environment variables.

Open `cmd/rp/main.go` and then launch the debugger from VSCode.

## Next: Testing locally

Move over to [testing locally]({{<ref testing-locally.md>}}).
