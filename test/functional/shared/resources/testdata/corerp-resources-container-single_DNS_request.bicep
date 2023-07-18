import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-container-single_DNS_request'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-single_DNS_request'
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
        source: 'http://containerb:3000'
      }
    }
  }
}
