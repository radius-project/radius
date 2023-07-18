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
  name: 'corerp-resources-container-two_containers_httproute'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-two_containers_httproute'
      }
    ]
  }
}

resource containera 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containera'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
    connections: {
      containerb: {
        source: containerbhttproute.id
      }
    }
  }
}

resource containerb 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerb'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
          provides: containerbhttproute.id
        }
      }
    }
  }
}

resource containerbhttproute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'containerbhttproute'
  location: location
  properties: {
    application: app.id
    port: port
  }
}
