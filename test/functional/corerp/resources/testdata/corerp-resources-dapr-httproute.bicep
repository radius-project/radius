import radius as radius

param location string = resourceGroup().location
param environment string
param magpieimage string


resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'dapr-invokehttproute'
  location: location
  properties: {
    environment: environment
  }
}

resource frontend 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'dapr-frontend'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      readinessProbe:{
        kind:'httpGet'
        containerPort:3000
        path: '/healthz'
      }
    }
    connections: {
      daprhttproute: {
        source: daprBackend.id
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

resource backend 'Applications.Core/containers@2022-03-15-privatepreview' = {
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
        appId: daprBackend.properties.appId
        appPort: 3000
        provides: daprBackend.id
      }
    ]
  }
}

resource daprBackend 'Applications.Connector/daprInvokeHttpRoutes@2022-03-15-privatepreview' = {
  name: 'dapr-backend-httproute'
  location: location
  properties: {
    environment: environment
    application: app.id
    appId: 'backend'
  }
}

