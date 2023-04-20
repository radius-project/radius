import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

resource env 'Applications.Core/environments@2023-04-15-preview' = {
  name: 'corerp-kmd-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      namespace: 'corerp-kmd-ns'
    }
  }
}

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-kmd-app'
  location: location
  properties: {
    environment: env.id
  }
}

resource container 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'corerp-kmd-ctnr'
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
    extensions: [
      {
        kind: 'kubernetesMetadata'
        annotations: {
          'user.cntr.ann.1': 'user.cntr.ann.val.1'
          'user.cntr.ann.2': 'user.cntr.ann.val.2'
        }
        labels: {
          'user.cntr.lbl.1': 'user.cntr.lbl.val.1'
          'user.cntr.lbl.2': 'user.cntr.lbl.val.2'
        }
      }
    ]
  }
}
