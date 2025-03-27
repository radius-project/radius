extension radius

param aciscope string = '/subscriptions/<>/resourceGroups/<>'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'radius-demo'
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
          templatePath: 'ghcr.io/radius-project/recipes/azure/rediscaches:latest'
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
  name: 'demo-app'
  properties: {
    environment: env.id
  }
}

resource redis 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'redis'
  properties: {
    environment: env.id
    application: app.id
  }
}


resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'gateway'
  properties: {
    application: app.id
    routes: [
      {
        path: '/'
        destination: 'http://frontend:3000'
      }
    ]
  }
}

resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'frontend'
  properties: {
    application: app.id
    container: {
      image: 'ghcr.io/radius-project/samples/demo:latest'
      ports: {
        web: {
          containerPort: 3000
        }
      }
    }
    connections: {
      redis: {
        source: redis.id
      }
      // backend1: {
      //   source: backend1.id
      // }
      // backend2: {
      //   source: backend2.id
      // }
    }
    extensions: [
      {
        kind:  'manualScaling'
        replicas: 2
      }
    ]
  }
}

// resource backend1 'Applications.Core/containers@2023-10-01-preview' = {
//   name: 'backend1'
//   properties: {
//     application: app.id
//     container: {
//       image: 'mcr.microsoft.com/azurelinux/base/nginx:1.25'
//       ports: {
//         api: {
//           containerPort: 80
//         }
//       }
//     }
//     extensions: [
//       {
//         kind:  'manualScaling'
//         replicas: 1
//       }
//     ]
//   }
// }

// resource backend2 'Applications.Core/containers@2023-10-01-preview' = {
//   name: 'backend2'
//   properties: {
//     application: app.id
//     container: {
//       image: 'mcr.microsoft.com/azurelinux/base/nginx:1.25'
//       ports: {
//         api: {
//           containerPort: 80
//         }
//       }
//     }
//     extensions: [
//       {
//         kind:  'manualScaling'
//         replicas: 2
//       }
//     ]
//   }
// }
