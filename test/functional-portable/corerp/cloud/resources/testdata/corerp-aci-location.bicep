extension radius

@description('Base name for resources')
param basename string

@description('The Azure resource group scope where ACI resources will be deployed')
param aciScope string = resourceGroup().id

// This template tests that ACI resources are created in the same location as the resource group
// Radius should automatically use the resource group's location for creating VNet, NSG, and Load Balancer resources
// instead of using a hardcoded location

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: '${basename}-env'
  properties: {
    compute: {
      kind: 'aci'
      resourceGroup: aciScope
      identity: {
        kind: 'systemAssigned'
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
  name: basename
  properties: {
    environment: env.id
  }
}

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'aci-location-gateway'
  properties: {
    application: app.id
    routes: [
      {
        path: '/'
        destination: 'http://aci-location-frontend:3000'
      }
    ]
  }
}

resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'aci-location-frontend'
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
      magpie: {
        source: magpie.id
      }
    }
    extensions: [
      {
        kind: 'manualScaling'
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

resource magpie 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'aci-location-magpie'
  properties: {
    application: app.id
    container: {
      image: 'ghcr.io/radius-project/magpiego:latest'
      env: {
        MAGPIE_PORT: {
          value: '8080'
        }
      }
    }
  }
}
