# Running Radius resource provider locally

There are many times where it's important to be able to debug the Radius RP locally:
- Fast inner loop debugging on a component in Radius.
- Can run a subset of processes required for a specific scenario (for example, just running Applications.Core and async processor)

Radius consists of a few processes that get deployed inside a Kubernetes cluster.

 This includes:

- Applications.Core RP (appcore-rp) - The RP, handles creation and management of Radius resources
- Universal Control Plane (ucp) - Acts as a proxy between the other services, also manages deployments of AWS resources
- Deployment Engine (bicep-de) - Handles deployment status for all resources and manages deployments of Azure resources

### Step 1: Running Applications.Core RP

Add the following to `configurations` in `.vscode/launch.json` in your Radius source directory.

```json
// .vscode/launch.json
{
    "name": "Applications.Core RP",
    "type": "go",
    "request": "launch",
    "mode": "debug",
    "program": "${workspaceFolder}/cmd/appcore-rp/main.go",
    "env": {
        "RADIUS_ENV": "self-hosted", // uses config from cmd/appcore-rp/radius-self-hosted.yaml
        "SKIP_AUTH": "true",
        "K8S_LOCAL": "true",
        "SKIP_ARM": "false",
        "ARM_AUTH_METHOD": "ServicePrincipal",
        "ARM_SUBSCRIPTION_ID": "<your-subscription-id>",
        "ARM_RESOURCE_GROUP": "<your-resource-group>",
        "AZURE_CLIENT_ID": "<your-sp-client-id>",
        "AZURE_CLIENT_SECRET": "<your-sp-client-secret>",
        "AZURE_TENANT_ID": "<your-sp-tenant-id>"
    }
}
```

Then, you can run and debug `Applications.Core RP` from VSCode.

With this configuration:
- Applications.Core RP will be running on port `8080`
- Applications.Connector RP will be running on port `8081`
- `etcd` will be used for storage

## Step 2: Running UCP

Add the following to `configurations` in `.vscode/launch.json` in your Radius source directory.

### launch.json
```json
// .vscode/launch.json
{
    "name": "UCP",
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
- UCP will be running on port `9000`
- `etcd` will be used for storage

## Step 3: Running Deployment Engine

Add the following to `configurations` in `.vscode/launch.json` in your Deployment Engine source directory.

```json
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
        "ASPNETCORE_URLS": "http://localhost:5017",
        "ASPNETCORE_ENVIRONMENT": "Development",
        "RADIUSBACKENDURL": "http://localhost:9000/apis/api.ucp.dev/v1alpha3"
    },
    "sourceFileMap": {
        "/Views": "${workspaceFolder}/Views"
    }
}
```

With this configuration:
- Deployment Engine will be running on port `5017`

### Step 4: Modifying the config.yaml to point to your local RP

You can use your build of `rad` (or build from source) to test against a local copy of the RP by creating a special environment.

To do this, open your Radius config file (`$HOME/.rad/config.yaml`) and edit it manually. 

You'll need to:

- Duplicate the contents of a workspace
- Give the new workspace a memorable name like `test` or `local`
- Add overrides for the UCP URL

**Example**

```yaml
workspaces:
  default: local
  items:
    existing:
      connection:
        context: your-context
        kind: kubernetes
      environment: /planes/radius/local/resourcegroups/your-resource-group/providers/applications.core/environments/your-environment
      scope: /planes/radius/local/resourceGroups/your-resource-group
      providerConfig:
        azure:
          subscriptionId: your-subscription-id
          resourceGroup: your-resource-group>

    # This is mostly a copy of `existing`
    local:
      connection:
        context: your-context
        kind: kubernetes
        # This is the part that you add
        overrides:
          ucp: http://localhost:9000
      environment: /planes/radius/local/resourcegroups/your-resource-group/providers/applications.core/environments/your-environment
      scope: /planes/radius/local/resourceGroups/your-resource-group
      providerConfig:
        azure:
          subscriptionId: your-subscription-id
          resourceGroup: your-resource-group
```

## Step 5: Register planes and create an environment

We need to initialize the `local` resource group in each of the `deployments/local` and `radius/local` planes. We also need to initialize an environment so that we can deploy Radius resources.

``` bash
# 1: Create local resource group in UCP deployments/local plane
curl --location --request PUT 'http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/deployments/local/resourceGroups/local' \
--header 'Content-Type: application/json' \
--data-raw '{}'

# 2: Create local resource group in UCP radius/local plane
curl --location --request PUT 'http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourceGroups/local' \
--header 'Content-Type: application/json' \
--data-raw '{}'

# 3: Create environment
curl --location --request PUT 'http://localhost:8080/planes/radius/local/resourceGroups/local/providers/Applications.Core/environments/your-environment?api-version=2022-03-15-privatepreview' \
--header 'Content-Type: application/json' \
--data-raw '{
    "location": "global",
    "properties": {
        "compute": {
            "kind": "kubernetes",
            "resourceId": "",
            "namespace": "default"
        }
    }
}
'
```

### Step 5: Run rad deploy

You can now run `rad deploy <bicep>` to deploy your Bicep to the local RP. You can also configure a launch.json file to debug the execution of `rad deploy`. FYI the args portion can be replaced with other rad commands (like `rad application list`) to test different CLI commands.

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
