import radius as radius

param aciscope string = '/subscriptions/66d1209e-1382-45d3-99bb-650e6bf63fc0/resourceGroups/cs2-demo-sun'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'radius-demo'
  properties: {
    compute: {
      kind: 'aci'
      resourceGroup: aciscope
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

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'demogateway'
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
          provides: gateway.id
        }
      }
    }
    connections: {
      backend: {
        source: accountsvc.id
      }
      metadatasvc: {
        source: metadatasvc.id
      }
    }
    extensions: [
      {
        kind:  'manualScaling'
        replicas: 3
      }
    ]
  }
}

resource accountsvc 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'accountsvc'
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
    extensions: [
      {
        kind:  'manualScaling'
        replicas: 3
      }
    ]
  }
}

resource metadatasvc 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'metadatasvc'
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
    extensions: [
      {
        kind:  'manualScaling'
        replicas: 2
      }
    ]
  }
}
