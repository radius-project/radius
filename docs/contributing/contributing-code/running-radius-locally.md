# Running Radius resource provider locally

There are many times where it's important to be able to debug the Radius RP locally:
- Fast inner loop debugging on a component in Radius.
- Can run a subset of processes required for a specific scenario (ex just running Applications.Core and async processor)

Currently in Radius, there are two different ways to run Radius locally based on whether we are running the _old_ way with a Custom RP, or in the _new_ world with the Application.Core RP.

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
  "name": "Run Radius rp",
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
      "ARM_RESOURCE_GROUP": "my-rg", // last three are optional based off which kind of environment you are running
      "ARM_SUBSCRIPTION_ID": "66d1209e-1382-45d3-99bb-650e6bf63fc0",
      "BASE_PATH": "/apis/api.radius.dev/v1alpha3"
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

### Step 2: Running the RP

Use `go run` to launch the RP from the same terminal where you configured the environment variables.

```sh
go run cmd/radius-rp/main.go
```

Or you can also run the RP from VSCode:
- With the `Run Radius RP` configuration in the launch.json file.
- Launch VSCode from the same terminal where you configurred the environment variables. Open `cmd/radius-rp/main.go` and then launch the debugger from VSCode.

### Step 3: Running the Deployment Engine

Next, run the Deployment Engine from a different terminal than the RP.

1. Install .NET 6: https://dotnet.microsoft.com/en-us/download/dotnet/6.0
1. Clone the Deployment Engine Repo: `git clone https://github.com/project-radius/deployment-engine`
1. Either run or Debug the Deployment Engine via:
```sh
# Set the backend URL that the Radius RP is currently running on, by default this will be what is below
export RADIUSBACKENDURL="http://localhost:5000/apis/api.radius.dev/v1alpha3"
dotnet run --project src/DeploymentEngine/DeploymentEngine.csproj
```

OR opening VSCode inside of the DeploymentEngine repo and adding the following configuration and run it:
```json
    "configurations": [
        {
            "name": ".NET Core Launch (web)",
            "type": "coreclr",
            "request": "launch",
            "preLaunchTask": "build",
            "program": "${workspaceFolder}/src/DeploymentEngine/bin/Debug/net6.0/arm-de.dll",
            "cwd": "${workspaceFolder}",
            "stopAtEntry": false,
            "console":"integratedTerminal",
            "serverReadyAction": {
                "action": "openExternally",
                "uriFormat": "http://localhost:%s/swagger/index.html",
                "pattern": "\\bNow listening on:\\s+(https?://\\S+)"
            },
            "env": {
                "ASPNETCORE_ENVIRONMENT": "Development",
                "RADIUSBACKENDURL": "http://localhost:5000/apis/api.radius.dev/v1alpha3" // Make sure this is the right URL for your RP
            },
            "sourceFileMap": {
                "/Views": "${workspaceFolder}/Views"
            },
            "launchSettingsProfile": "DeploymentEngine",
            "launchSettingsFilePath": "${workspaceFolder}/src/DeploymentEngine/Properties/launchSettings.json"
        }
    ]
```

By default, the Deployment Engine will run on port 5017 for http

### Step 4: Modifying the config.yaml to point to your local RP

You can use your build of `rad` (or build from source) to test against a local copy of the RP by creating a special environment.

To do this, open your environment file (`$HOME/.rad/config.yaml`) and edit it manually. 

You'll need to:

- Duplicate the contents of the environment (whether it be kubernetes, dev, or azure)
- Give the new environment a memorable name like `test` or `local`
- Add `radiusrplocalurl` and `deploymentenginelocalurl` to point to the URLs of your local RP and DE

**Example**

```yaml
environment:
  default: local
  items:
    local:
      context: justin-d
      kind: kubernetes
      namespace: default
      radiusrplocalurl: http://localhost:5000
      deploymentenginelocalurl: http://localhost:5017
```

### Step 5: Run rad deploy

You can now run `rad deploy <bicep>` to deploy your BICEP to the local RP. You can also configure a launch.json file to debug the execution of `rad deploy`.

```json
{
    "name": "Launch rad deploy",
    "type": "go",
    "request": "launch",
    "mode": "debug",
    "program": "${workspaceFolder}/cmd/rad/main.go",
    "args": ["deploy", "<bicep>"]
},
```

## New world - Application.Core RP, Deployment Engine, UCP

The new world consists of a few more operations and processes that are unique. This includes:

UCP - Universal Control Plane, we need to run this and register planes with the RP.
Application.Core - The new RP that we're building.

### Step 1: Running Applications.Core RP

Running the Applications requires the following configuration before running:

```sh
export RADIUS_ENV="self-hosted-dev"
go run cmd/appcore-rp/main.go
```

Or in VSCode by adding this to the launch.json file in the Radius repository:

```json
{
    "name": "Run appcore controller",
    "type": "go",
    "request": "launch",
    "mode": "debug",
    "program": "${workspaceFolder}/cmd/appcore-rp/main.go",
    "env": {
        "RADIUS_ENV": "self-hosted-dev"
    }
},
```

With this configuration:
- Port used will be `8080`
- etcd will be used for storage


### Step 2: Running the Deployment Engine

Next, run the Deployment Engine from a different terminal than the RP.

1. Install .NET 6: https://dotnet.microsoft.com/en-us/download/dotnet/6.0
1. Clone the Deployment Engine Repo: `git clone https://github.com/project-radius/deployment-engine`
1. Either run or Debug the Deployment Engine via:
```sh
# Set the backend URL to **UCP** URL that it will be running on by default this will be what is below
# TODO RADIUSBACKENDURL should be renamed to something UCP like.
export RADIUSBACKENDURL="http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local"
dotnet run --project src/DeploymentEngine/DeploymentEngine.csproj
```

OR opening VSCode inside of the DeploymentEngine repo and adding the following configuration to launch.json file:
```json
    "configurations": [
        {
            "name": ".NET Core Launch (web)",
            "type": "coreclr",
            "request": "launch",
            "preLaunchTask": "build",
            "program": "${workspaceFolder}/src/DeploymentEngine/bin/Debug/net6.0/arm-de.dll",
            "cwd": "${workspaceFolder}",
            "stopAtEntry": false,
            "console":"integratedTerminal",
            "serverReadyAction": {
                "action": "openExternally",
                "uriFormat": "http://localhost:%s/swagger/index.html",
                "pattern": "\\bNow listening on:\\s+(https?://\\S+)"
            },
            "env": {
                "ASPNETCORE_ENVIRONMENT": "Development",
                "RADIUSBACKENDURL": "http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local" // Make sure this is the right URL for your RP
            },
            "sourceFileMap": {
                "/Views": "${workspaceFolder}/Views"
            }
        }
    ]
```

### Step 3: Running UCP

UCP is a part of the Radius repo. To run it, first make sure `cmd/ucpd/ucp-self-hosted-dev.yaml` is updated with the right URLs for each plane:

```yaml
# This is an example of configuration file.
storageProvider:
  #Uncomment to use the etcd store. Right now this only supports running in-memory (not for production use).
  provider: "etcd"
  etcd:
   inmemory: true

#Default planes configuration with which ucp starts
planes:
  - id: "/planes/radius/local"
    properties:
      resourceProviders:
        Applications.Core: "http://localhost:8080" # Make sure these URLs point to the right URLs
        Applications.Connector: "http://localhost:8080"
  - id: "/planes/deployments/local"
    properties:
      resourceProviders:
        Microsoft.Resources: "http://localhost:5017" # URL for the Deployment Engine
```

Then execute the following:

```sh
export BASE_PATH="/apis/api.ucp.dev/v1alpha3"
export PORT="9000"
export UCP_CONFIG="cmd/ucpd/ucp-self-hosted-dev.yaml"
go run cmd/ucp/main.go
```

Or having the following VSCode configuration in the radius repository launch.json file:
```json
{
    "name": "Run ucp controller",
    "type": "go",
    "request": "launch",
    "mode": "debug",
    "program": "${workspaceFolder}/cmd/ucpd/main.go",
    "env": {
        "UCP_CONFIG": "${workspaceFolder}/cmd/ucpd/ucp-self-hosted-dev.yaml",
        "PORT": "9000",
        "BASE_PATH": "/apis/api.ucp.dev/v1alpha3"
    }
},
```

With this configuration:
- Port used will be `9000`
- etcd will be used for storage

### Step 4: Modifying the config.yaml to point to your local RP

You can use your build of `rad` (or build from source) to test against a local copy of the RP by creating a special environment.

To do this, open your environment file (`$HOME/.rad/config.yaml`) and edit it manually. 

You'll need to:

- Duplicate the contents of a environment
- Give the new environment a memorable name like `test` or `local`
- Add `ucplocalurl` and `enableucp` to point to the URLs of your local UCP and enabling UCP

**Example**

```yaml
environment:
  default: local
  items:
    local:
      context: justin-d
      kind: kubernetes
      namespace: default
      enableucp: true
      ucplocalurl: http://localhost:9000
```

### Step 4.5: Have the right verison of bicep installed and right type of Bicep file

To be able to deploy to the appcore RP, we need to have the right version of bicep installed.

```bash
make build
./dist/.../rad bicep download # path to rad that is built as part of the repo
```

```bicep
import radius as radius {
  foo: 'foo'
}

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'myenv'
  location: 'westus2'
  properties: {
    compute:{
      kind: 'kubernetes'
      resourceId: ''
    }
  }
}
```

### Step 5: Run rad deploy

You can now run `rad deploy <bicep>` to deploy your BICEP to the local RP. You can also configure a launch.json file to debug the execution of `rad deploy`. FYI the args portion can be replaced with other rad commands (like `rad application list`) to test different CLI commands.

```json
{
    "name": "Launch rad deploy",
    "type": "go",
    "request": "launch",
    "mode": "debug",
    "program": "${workspaceFolder}/cmd/rad/main.go",
    "args": ["deploy", "<path to bicep file to deploy>"]
},
```
