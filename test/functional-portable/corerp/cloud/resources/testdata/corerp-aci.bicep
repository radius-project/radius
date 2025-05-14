extension radius

@description('Specifies the scope of azure resources.')
param aciScope string = resourceGroup().id
@description('The managed identity to use to authenticate with NGroups.')
param managedIdentity string 

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'aci-env'
  properties: {
    compute: {
      kind: 'aci'
      resourceGroup: aciScope
      identity: {
        kind:'managedIdentity'
        managedIdentity: [managedIdentity]
      }
    }
    recipes: {
      'Applications.Datastores/redisCaches': {
        default: {
          templateKind: 'bicep'
          plainHttp: true
          templatePath: 'ghcr.io/radius-project/recipes/azure/rediscaches:latest'
        }
      }
    }
    providers: {
      azure: {
        scope: aciScope
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'aci-app'
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
    // connections: {
    //   magpie: {
    //     source: magpie.id
    //   }
    // }
    extensions: [
      {
        kind:  'manualScaling'
        replicas: 2
      }
    ]
    runtimes: {
      aci: {
        gatewayID: gateway.id
      }
    }
  }
}

// resource magpie 'Applications.Core/containers@2023-10-01-preview' = {
//   name: 'magpie'
//   properties: {
//     application: app.id
//     container: {
//       image: 'ghcr.io/radius-project/magpiego:latest'
//       env: {
//         MAGPIE_PORT: {
//           value: '8080'
//         }
//       }
//     }
//   }
// }
