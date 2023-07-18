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

resource containerer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerer'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
    connections: {
      containervb: {
        // source: containerhttproute.id
        source: 'http://containervb:3000'
      }
    }
  }
}

resource containervb 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containervb'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
          // adds label to container that links it to the httproute
          // provides: containerhttproute.id
        }
      }
    }
  }
}
