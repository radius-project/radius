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
  name: 'corerp-app-rte-kme'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-ns-rte-kme-app'
      }
      {
        kind: 'kubernetesMetadata'
        annotations: {
          'user.ann.1': 'user.ann.val.1'
          'user.ann.2': 'user.ann.val.2'
        }
        labels: {
          'user.lbl.1': 'user.lbl.val.1'
          'user.lbl.2': 'user.lbl.val.2'
        }
      }
    ]
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'ctnr-rte-kme-ctnr'
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

resource httproute 'Applications.Core/httpRoutes@2023-10-01-preview' = {
  name: 'ctnr-rte-kme'
  location: location
  properties: {
    application: app.id
    port: port
  }
}
