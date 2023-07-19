import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-container-single-dns-request'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-single-dns-request'
      }
    ]
  }
}

resource containerqo 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerqo'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
    connections: {
      containerqp: {
        source: 'http://containerqp:3000'
      }
    }
  }
}
