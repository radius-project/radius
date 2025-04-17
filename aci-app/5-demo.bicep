extension radius

param aciscope string = '/subscriptions/66d1209e-1382-45d3-99bb-650e6bf63fc0/resourceGroups/shruthikumar'

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

// resource redis 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
//   name: 'redis'
//   properties: {
//     environment: env.id
//     application: app.id
//   }
// }


// resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
//   name: 'gateway'
//   properties: {
//     application: app.id
//     routes: [
//       {
//         path: '/'
//         destination: 'http://frontend:3000'
//       }
//     ]
//   }
// }

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
    runtimes: { 
      aci: {
        containerGroupProfile: {
        }
      }
    }
  }
}
