import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-container-httproute'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-httproute-app'
      }
    ]
  }
}

resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'ctnr-rte-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        rteUrl: httproute.properties.url
      }
      ports: {
        web: {
          containerPort: port
          provides: httproute.id
        }
      }
    }
    connections: {}
  }
}

resource httproute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'ctnr-rte-rte'
  location: location
  properties: {
    application: app.id
    port: port
  }
}
