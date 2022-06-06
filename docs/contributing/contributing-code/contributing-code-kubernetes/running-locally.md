# Running Radius resource provider locally with a Kubernetes Environment

There are many times where it's important to be able to debug the Radius RP locally, as there may be code that needs to be updated.

Currently in Radius, there are two different ways to run Radius locally based on whether we are running the _old_ way with a Custom RP, or in the new world with the Application.Core RP.

## Old world - Custom RP, Mongo, and Deployment Engine

You can run the Radius RP locally using:

- MongoDB in a container
- Your local Kubernetes credentials

This will enable to use an ephemeral database as well as your local changes to the RP (built from source).

### Step 1: Running MongoDB

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

### Configuring the RP

**TLDR:**

```sh
export SKIP_AUTH='true'
export PORT='5000'
export MONGODB_CONNECTION_STRING='mongodb://mongoadmin:secret@mongo:27017/rpdb?authSource=admin'
export MONGODB_DATABASE='rpdb'
export K8S_LOCAL='true'
export ARM_RESOURCE_GROUP="$(whoami)-radius"
export ARM_SUBSCRIPTION_ID="$(az account show --query 'id'  --output tsv)"
```

Configures all of the required environment variables:

- Listening on port 5000
- Using a local MongoDB container
- Using your local Kubernetes configuration
- Using your local `az` CLI config to talk to a specified resource group

You can also specify these in your launch.json settings:

```json
{
  "name": "Run rp",
  "type": "go",
  "request": "launch",
  "mode": "debug",
  "program": "${workspaceFolder}/cmd/radius-rp/main.go",
  "env": {
      "PORT": "5000",
      "SKIP_AUTH": "true",
      "MONGODB_CONNECTION_STRING": "mongodb://mongoadmin:secret@mongo:27017/rpdb?authSource=admin",
      "MONGODB_DATABASE": "rpdb",
      "K8S_LOCAL": "true",
      "ARM_RESOURCE_GROUP": "my-rg",
      "ARM_SUBSCRIPTION_ID": "66d1209e-1382-45d3-99bb-650e6bf63fc0",
      // "K8S_CLUSTER_NAME": "radius-aks-ya7cxvgdeh6su",
      // "K8S_RESOURCE_GROUP": "justin-validation-011",
      // "K8S_SUBSCRIPTION_ID": "66d1209e-1382-45d3-99bb-650e6bf63fc0",
      // "BASE_PATH": "/apis/api.radius.dev/v1alpha3"
  }
},
```

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

### Step 3: Running the RP

Use `go run` to launch the RP from the same terminal where you configured the environment variables.

```sh
go run cmd/rp/main.go
```

### Step 4: Running the Deployment Engine



## Optional: Debugging the RP

Launch VSCode from the same terminal where you configurred the environment variables.

Open `cmd/rp/main.go` and then launch the debugger from VSCode.

## New world - Application.Core RP, Deployment Engine, UCP



# Test Radius locally

Testing the RP locally can be challenging because the Radius RP is just one part of a distributed system. The actual processing of ARM templates (the output of a `.bicep file`) is handled by the ARM deployment engine, not us.


## Pattern for integration testing

As a general pattern, you can find example applications in the `/examples` folder. Each folder has a `template.bicep` file which contains a deployable application.

If you are building new features, or want to test deployment interactions the best way is to either:

- Make a series of deploy and delete operations with one of these example applications
- Write a new example application

## Local testing with rad

You can use your build of `rad` (or build from source) to test against a local copy of the RP by creating a special environment.

To do this, open your environment file (`$HOME/.rad/config.yaml`) and edit it manually. 

You'll need to:

- Duplicate the contents of an Azure Cloud environment
- Give the new environment a memorable name like `test` or `local`
- Change the environment kind from `azure` to `localrp`
- Add a `url` property with the URL of your local RP

**Before**

```yaml
environment:
  default: my-cool-env
  items:
    my-cool-env:
      clustername: radius-aks-j5oqzddqmf36s
      kind: azure
      resourcegroup: my-cool-env
      controlplaneresourcegroup: RE-my-cool-env
      subscriptionid: 66d1209e-1382-45d3-99bb-650e6bf63fc0
```

**After**

```yaml
environment:
  default: my-cool-env
  items:
    local:
      clustername: radius-aks-j5oqzddqmf36s
      context: radius-aks-j5oqzddqmf36s
      namespace: default
      kind: localrp # remember to set the kind
      url: http://localhost:5000 # use whatever port you prefer when running the RP locally
      resourcegroup: my-cool-env
      controlplaneresourcegroup: RE-my-cool-env
      subscriptionid: 66d1209e-1382-45d3-99bb-650e6bf63fc0
    my-cool-env:
      clustername: radius-aks-j5oqzddqmf36s
      context: radius-aks-j5oqzddqmf36s
      namespace: default
      kind: azure
      resourcegroup: my-cool-env
      subscriptionid: 66d1209e-1382-45d3-99bb-650e6bf63fc0
```

Now you can run `rad env switch local` and use this environment just like you'd use any other.

To run a local RP deployment, a Deployment Engine will run either:
- Automatically on a random port for the duration of the deployment
- Specifying a URL that the Deployment Engine will connect to with `apideploymentenginebaseurl` in the localrp environment section.
