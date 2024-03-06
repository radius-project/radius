import radius as radius

param location string = resourceGroup().location
param environment string
param magpieimage string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'dapr-serviceinvocation'
  location: location
  properties: {
    environment: environment
  }
}

resource frontend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'dapr-frontend'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        // Used by magpie to communicate with the backend.
        CONNECTION_DAPRHTTPROUTE_APPID: 'backend'
      }
      readinessProbe:{
        kind:'httpGet'
        containerPort:3000
        path: '/healthz'
      }
    }
    extensions: [
      {
        kind: 'daprSidecar'
        appId: 'frontend'
      }
    ]
  }
}

resource backend 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'dapr-backend'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        orders: {
          containerPort: 3000
        }
      }
      readinessProbe:{
        kind:'httpGet'
        containerPort:3000
        path: '/healthz'
      }
    }
    extensions: [
      {
        kind: 'daprSidecar'
        appId: 'backend'
        appPort: 3000
      }
    ]
  }
}

