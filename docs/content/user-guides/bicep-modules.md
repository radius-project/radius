---
type: docs
title: "Separate your application components into Bicep modules"
linkTitle: "Break into modules"
description: "Learn how to grow a single-file Radius application into a multi-file, large scale application with Bicep modules."
weight: 400
---

So far you've been creating and deploying Radius applications that are a single file. For larger application you'll want to break your appliction into multiple files, each representing a separate microservice. [Bicep modules](https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/modules) provides this capability.

## Start with your application

For this example, we'll be using a frontend/backend Radius application with a database that we want to break into separate files:

```bicep
resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'
  
  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'nginx:latest'
      }
    }
  }

  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      container: {
        image: 'nginx:latest'
      }
    }
  }

  resource db 'mongo.com.mongodb' = {
    name: 'db'
    properties: {
      config: {
        managed: true
      }
    }
  }
}
```

### Break into files

Begin my breaking your application into separate Bicep files:

{{< tabs "app.bicep" "frontend.bicep" "backend.bicep" "infra.bicep" >}}

{{% codetab %}}
```bicep
resource myapp 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'
  
  module frontendModule 'frontend.bicep' = {
    name: 'frontend'
    params: {
      backendPort: backendModule.outputs.backend.port
    }
  }

  module backendModule 'backend.bicep' = {
    name: 'backend'
  }

  module dbModule 'mongo.com.mongodb' = {
    name: 'db'
  }
}
```
{{% /codetab %}}

{{% codetab %}}
```bicep
resource frontendModule 'ContainerComponent' = {
  name: 'frontend'
  properties: {
    container: {
      image: 'nginx:latest'
    }
  }
}

output frontend object = frontendModule
```
{{% /codetab %}}

{{% codetab %}}
```bicep
resource backendModule 'ContainerComponent' = {
  name: 'backend'
  properties: {
    container: {
      image: 'nginx:latest'
    }
  }
}

output backend object = backendModule
```
{{% /codetab %}}

{{% codetab %}}
```bicep
resource dbModule 'mongo.com.mongodb' = {
  name: 'db'
  properties: {
    config: {
      managed: true
    }
  }
}

output db object = dbModule
```
{{% /codetab %}}

{{< /tabs >}}

## TODO: complete this guide
