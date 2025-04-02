extension radius

param aciscope string = '/subscriptions/<>/resourceGroups/<>'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'aci-environment'
  properties: {
    compute: {
      kind: 'aci'
      resourceGroup: aciscope
    }
    recipes: {
      'Applications.Datastores/redisCaches':{
        default: {
          templateKind: 'bicep'
          plainHttp: true
          templatePath: 'ghcr.io/radius-project/recipes/azure/rediscaches:0.30.0-rc3'
        }
      }
    }
    providers: {
      azure: {
        scope: aciscope
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'aci-application'
  properties: {
    environment: env.id
  }
}

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'gateway'
  properties: {
    application: app.id
    routes: [
      {
        path: '/'
        destination: 'http://demo:3000'
      }
    ]
  }
}

resource demo 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'demo'
  properties: {
    application: app.id
    container: {
      image: 'ghcr.io/radius-project/samples/demo:latest'
      ports: {
        web: {
          containerPort: 3000
          // provides: gateway.id
        }
      }
    }
    connections: {
      mongodb: {
        source: redis.id
      }
      backend: {
        source: backend.id
      }
    }
  }
}

resource redis 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'redis'
  properties: {
    environment: env.id
    application: app.id
  }
}

resource backend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'backend'
  properties: {
    application: app.id
    container: {
      image: 'nginx:latest'
      ports: {
        api: {
          containerPort: 80
        }
      }
    }
  }
}
