import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-container-podpatch'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-podpatch'
      }
    ]
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'ctnr-podpatch'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
        }
      }
    }
    connections: {}
    runtimes: {
      kubernetes: {
        pod: {
          containers: [
            {
              name: 'log-collector'
              image: 'ghcr.io/radius-project/fluent-bit:2.1.8'
            }
          ]
          hostNetwork: true
        }
      }
    }
  }
}
