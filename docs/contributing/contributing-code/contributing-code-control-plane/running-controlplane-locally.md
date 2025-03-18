# Running Radius control plane provider locally

Radius consists of a few processes that get deployed inside a Kubernetes cluster.

 This includes:

- Applications.Core RP / Portable Resources' Providers (applications-rp) - The resource provider that handles processing of core resources as well as recipes.
- Universal Control Plane (ucp) - Acts as a proxy between the other services, also manages deployments of AWS resources.
- Deployment Engine (bicep-de) - Handles deployment orchestration for bicep files.

The easiest way to get started is to launch Radius using VS Code. This will give you the ability to debug all of the processes. This workflow will run all of the Radius processes locally on your computer without containerizing them.

> ⚠️ The debugging setup provided by these instructions **does NOT** share its database with an installed copy of Radius. It will use a separate namespace to store data.

## Endpoints

If you need to manually test APIs you can reach them at the following endpoints after following these instructions:

- UCP: port 9000
- Applications.Core RP / Portable Resources' Providers (applications-rp): port 8080
- Deployment Engine: port 5017

## Prerequisites

1. Create a Kubernetes cluster, or set your current context to a cluster you want to use. The debug configuration will use your current cluster for storing data. 
2. (Optional) Configure any cloud provider credentials you want to use for developing Radius.
  
> 💡 Radius will use your locally configured Azure or AWS credentials. If you are able to use the `az` or `aws` CLI then you don't need to do any additional setup.

## Setup Step 1: Run `rad init`

Run one of the following two commands:

```sh
# Choose this by default
rad init

# Choose this if you want to do advanced setup
rad init --full
```

This will install Radius and configure an environment for you. The database that's used **will NOT** be shared with your debug setup, so it mostly doesn't matter what choices you make.


## Setup Step 2: Modify config.yaml to point to your local RPs

You can use your build of `rad` (or build from source) to test against a local copy of the RP by creating a special workspace configuration.

To do this, open your Radius config file (`$HOME/.rad/config.yaml`) and edit it manually.

Your configuration file probably looks like this:

```yaml
workspaces:
  default: default
  items:
    default:
      connection:
        context: kind-kind
        kind: kubernetes
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
      scope: /planes/radius/local/resourceGroups/default
```

Make a copy of the `default` workspace called `dev` and set it as the default. Then add the `overrides` section from the example below. 

 This example adds a `dev` workspace:

```yaml
workspaces:
  default: dev
  items:
    dev:
      connection:
        context: kind-kind
        kind: kubernetes
        overrides:
          ucp: http://localhost:9000
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
      scope: /planes/radius/local/resourceGroups/default
    default:
      connection:
        context: kind-kind
        kind: kubernetes
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
      scope: /planes/radius/local/resourceGroups/default
```

The `overrides` element tells the `rad` CLI what endpoint to talk to.

## Setup Step 3: Create radius-testing namespace

Run this command to create the namespace that will be used to store data.

```sh
kubectl create namespace radius-testing
```

## Setup Step 4: Setup Deployment Engine 

> 💡 This way of setting up deployment-engine is useful if you are an external contributor and do not have access to `radius-project/deployment-engine` repo.

> 💡 If you have access to the deployment-engine repository and would like to debug it, you can omit this step and proceed to step 5.

### Setup Docker

If Docker is not already installed, 
* Download and install it from the [Docker Desktop download page](https://www.docker.com/products/docker-desktop). 
Choose the installer that matches your operating system.
* Open a terminal and run the following command to verify that Docker is installed and running:
```sh
docker --version
```
You should see the Docker version information.

### Run Deployment Engine as a Docker container

Run the below command.

```sh
docker run -e RADIUSBACKENDURL=http://host.docker.internal:9000/apis/api.ucp.dev/v1alpha3 -p 5017:8080 ghcr.io/radius-project/deployment-engine:latest
```

`host.docker.internal` is a special DNS name provided by Docker that allows containers to access services running on the host machine 

### Update launch.json 

Open launch.json and comment out `Launch Deployment Engine` in `Launch Control Plane (all)`. The debug setup will use the Deployment Engine running as docker container. 
  ```json
  "compounds": [
    {
      "name": "Launch Control Plane (all)",
      "configurations": [
        "Launch UCP",
        "Launch Applications RP",
        "Launch Dynamic RP",
        "Launch Controller",
        // "Launch Deployment Engine"
      ],
      "stopAll": true
    }
  ],
  ```

## Setup Step 5 (optional): Setup deployment Engine for debugging


> 💡 The Bicep deployment engine uses .NET. However you don't need to know C# or .NET to develop locally with Radius.

If you have access to `radius-project/deployment-engine` repo, you can follow the steps below to set up Deployment Engine for debugging instead of running it in a docker container.
1. Clone the `radius-project/radius` and `radius-project/deployment-engine` repos next to each other.
2. Run `git submodule update --init` in the `deployment-engine` repo.
3. Install .NET 8.0 SDK - <https://dotnet.microsoft.com/en-us/download/dotnet/8.0>.
4. Install C# VS Code extension - <https://marketplace.visualstudio.com/items?itemName=ms-dotnettools.csharp>.
5. Uncomment // "Launch Deployment Engine" in launch.json if it's commented out.



## Setup Step 6: Create Resource Group and Environment

At this point Radius is working but you don't have a resource group or environment. You can launch Radius and then use the CLI to create these.

In VS Code:

- Open the Debug tab in VS Code
- Select `Launch Control Plane (all)` from the drop-down
- Press Debug

Wait until all five debuggers have attached and their startup sequences have completed. You should see the following entries in the Debug Tab --> Call Stack window:

- Deployment Engine
- UCP
- Applications RP
- Dynamic RP
- Controller

Then at the command line run:

```sh
rad group create default
rad env create default
```

At this point you're done with setup! Feel free to stop the debugger.

## Troubleshooting

### I got an error saying I need to clone the deployment engine

> The radius-project/deployment-engine is not cloned as a sibling to the Radius repo. Please clone the radius-project/deployment-engine repo next to the Radius repo and try again.

You should be able to successfully the following commands from the Radius repository root:

```sh
ls ../deployment-engine/src
ls ../deployment-engine/submodules/bicep-extensibility/src
```

If these commands fail, please re-read the prerequisites related to cloning the deployment engine.

### I got an error related to missing dotnet or missing .NET SDK

Make sure that `dotnet` is on your path. If you just installed .NET then you might need to reopen VS Code and your terminal.

If `dotnet` is on your path you should be able to run the following commands:

```sh
dotnet --list-runtimes
dotnet --list-sdks
```

Make sure you see a `8.0` entry in `--list-runtimes` for `Microsoft.AspNetCore.App` and a `8.0` or newer entry for `--list-sdks`.

If you run into issues here, please re-read the prerequisites related to installing .NET.

### I got a "InvalidTemplate" error when deploying a bicep file

> sample error message:
```json
{
  "code": "InvalidTemplate",
  "message": "Deployment template validation failed: 'The template language version '2.1-experimental' is not recognized.'.",
}
```

Pull latest of the `radius-project/deployment-engine` project.
Run submodule update to update bicep extensibility support for extensible resources:

```bash
git submodule update --init --recursive
```

Build deployment-engine project

```bash
dotnet build
```

After building the Deployment Engine, build the radius project and redeploy bicep file by running steps from [Debugging](#debugging).