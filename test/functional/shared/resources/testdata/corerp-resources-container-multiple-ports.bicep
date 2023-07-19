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
  name: 'corerp-resources-container-multiple-ports'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-multiple-ports'
      }
    ]
  }
}

resource containerqt 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerqt'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
        }
        wonderland: {
          containerPort: 42
        }
        vegas: {
          containerPort: 777
        }
      }
    }
  }
}
