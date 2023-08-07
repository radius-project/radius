import radius as radius

param location string = resourceGroup().location
param environment string
param magpieimage string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'dapr-si-old'
  location: location
  properties: {
    environment: environment
  }
}

resource frontend 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'dapr-frontend-old'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        // Used by magpie to communicate with the backend.
        CONNECTION_DAPRHTTPROUTE_APPID: 'backend-old'
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
        appId: 'frontend-old'
      }
    ]
  }
}

resource backend 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'dapr-backend-old'
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
        appId: 'backend-old'
        appPort: 3000
      }
    ]
  }
}

