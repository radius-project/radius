# Running Radius control plane provider locally

There are many times where it's important to be able to debug the Radius RP locally:
- Fast inner loop debugging on a component in Radius.
- Can run a subset of processes required for a specific scenario (for example, just running Applications.Core and async processor)

Radius consists of a few processes that get deployed inside a Kubernetes cluster.

 This includes:

- Applications.Core RP / Applications.Link RP (appcore-rp) - The resource provider that handles processing of core resources as well as recipes.
- Universal Control Plane (ucp) - Acts as a proxy between the other services, also manages deployments of AWS resources.
- Deployment Engine (bicep-de) - Handles deployment orchestration for bicep files.

The easiest way to get started to to launch Radius using VS Code. This will give you the ability to debug all of the processes. This workflow will run all of the Radius processes locally on your computer without containerizing them.

## Endpoints

If you need to manually test APIs you can reach them at the following endpoints following these instructions:

- UCP: port 9000
- AppCore RP: port 8080
- AppLink RP: port 8081
- Deployment Engine: port 5017

## Prerequisites

1. Create a Kubernetes cluster, or set your current context to a cluster you want to use. The debug configuration will use your current cluster for storing data. 
2. Clone the `project-radius/radius` and `project-radius/deployment-engine` repo next to each other. 
3. Run `git submodule update --init` in the `deployment-engine` repo
4. Install .NET 6.0 SDK - https://dotnet.microsoft.com/en-us/download/dotnet/6.0
5. (Optional) Configure any cloud provider credentials you want to use for developing Radius. 
  
> 💡 The Bicep deployment engine uses .NET. However you don't need to know C# or .NET to develop locally with Radius.

> 💡 Radius will use your locally configured Azure or AWS credentials. If you are able to use the `az` or `aws` CLI then you don't need to do any additional setup.

## Setup Step 1: Run `rad init`

Run one of the following two commands:

```sh
# Choose this by default
rad init --dev

# Choose this if you want to do advanced setup
rad init
```

This will install Radius and configure an environment for you. The database that's used will be shared between the instance of Radius installed in your Kubernetes cluster, and the instance running locally.

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

Make a copy of the `default` environment called `dev` and set it as the default. Then add the `overrides` section from the example below. 

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


## Debugging

Now you can launch the Radius locally through the VSCode menu.

- Open the Debug tab in VS Code
- Select `Launch Control Plane (all)` from the drop-down
- Press Debug
- You're up and running!

## Troubleshooting

### I got an error saying I need to clone the deployment engine

> The project-radius/deployment-engine is not cloned as a sibling to the radius repo. Please clone the project-radius/deployment-engine repo next to the Radius repo and try again.

You should be to successfully the following commands from the Radius repository root:

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

Make sure you see a `6.0` entry in `--list-runtimes` for `Microsoft.AspNetCore.App` and a `6.0` or newer entry for `--list-sdks`.

If you run into issues here, please re-read the prerequisites related to installing .NET.