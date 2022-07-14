# Running Radius resource provider locally

There are many times where it's important to be able to debug the Radius RP locally:
- Fast inner loop debugging on a component in Radius.
- Can run a subset of processes required for a specific scenario (ex just running Applications.Core and async processor)

Radius consists of a few processes that get deployed inside a Kubernetes cluster.

 This includes:

- UCP - Universal Control Plane, we need to run this and register planes with the RP.
- Application.Core - The new RP that we're building.
- Bicep Deployment Engine

### Step 1: Running Applications.Core RP

Running the Applications.Core RP requires the following configuration before running:

```sh
export RADIUS_ENV="self-hosted"
cd cmd/appcore-rp
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
        "RADIUS_ENV": "self-hosted"
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
      kind: "UCPNative"
  - id: "/planes/deployments/local"
    properties:
      resourceProviders:
        Microsoft.Resources: "http://localhost:5017" # URL for the Deployment Engine
      kind: "UCPNative"
```

Then execute the following:

```sh
export BASE_PATH="/apis/api.ucp.dev/v1alpha3"
export PORT="9000"
export UCP_CONFIG="cmd/ucpd/ucp-self-hosted-dev.yaml"
cd cmd/ucpd
go run cmd/ucpd/main.go
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

To do this, open your Radius config file (`$HOME/.rad/config.yaml`) and edit it manually. 

You'll need to:

- Duplicate the contents of a workspace
- Give the new workspace a memorable name like `test` or `local`
- Add overrides for the URLs of all of these services

**Example**

```yaml
workspaces:
  default: local
  items:
    existing:
      connection:
         kind: kubernetes
         context: justin-d
      scope: /planes/radius/local/resourceGroups/justin-d
      environment: /planes/radius/local/resourceGroups/justin-d/providers/Applications.Core/environments/cool-test

    # This is mostly a copy of `existing`
    local:
      connection:
        kind: kubernetes
        context: justin-d
        # This is the part you add!
        override:
          ucp: http://localhost:9000
      scope: /planes/radius/local/resourceGroups/justin-d
      environment: /planes/radius/local/resourceGroups/justin-d/providers/Applications.Core/environments/cool-test
        
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
      namespace: 'myenv'
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
