import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-kmd-cascade-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      namespace: 'corerp-kmd-cascade-ns'
    }
    extensions: [
      {
        kind: 'kubernetesMetadata'
        annotations: {
          'user.env.ann.1': 'user.env.ann.val.1'
          'user.env.ann.2': 'user.env.ann.val.2'
          // reserved key prefix check
          'radius.dev/env.ann.1': 'reserved.ann.val.1'
          // collision check
          'collision.ann.1': 'collision.env.ann.val.1'
          'collision.env.app.ann.1': 'collision.env.ann.val.1'
        }
        labels: {
          'user.env.lbl.1': 'user.env.lbl.val.1'
          'user.env.lbl.2': 'user.env.lbl.val.2'
          'radius.dev/env.lbl.1': 'reserved.lbl.val.1'
          'collision.lbl.1': 'collision.env.lbl.val.1'
        }
      }
    ]
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-kmd-cascade-app'
  location: location
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesMetadata'
        annotations: {
          'user.app.ann.1': 'user.app.ann.val.1'
          'user.app.ann.2': 'user.app.ann.val.2'
          'radius.dev/app.ann.1': 'reserved.ann.val.1'
          'collision.ann.1': 'collision.app.ann.val.1'
          'collision.env.app.ann.1': 'collision.app.ann.val.1'
        }
        labels: {
          'user.app.lbl.1': 'user.app.lbl.val.1'
          'user.app.lbl.2': 'user.app.lbl.val.2'
          'radius.dev/app.lbl.1': 'reserved.lbl.val.1'
          'collision.lbl.1': 'collision.app.lbl.val.1'
          'collision.app.cntr.lbl.1': 'collision.app.lbl.val.1'
        }
      }
    ]
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'corerp-kmd-cascade-ctnr'
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
          'radius.dev/cntr.ann.1': 'reserved.ann.val.1'
          'collision.ann.1': 'collision.cntr.ann.val.1'
        }
        labels: {
          'user.cntr.lbl.1': 'user.cntr.lbl.val.1'
          'user.cntr.lbl.2': 'user.cntr.lbl.val.2'
          'radius.dev/cntr.lbl.1': 'reserved.lbl.val.1'
          'collision.lbl.1': 'collision.cntr.lbl.val.1'
          'collision.app.cntr.lbl.1': 'collision.cntr.lbl.val.1'
        }
      }
    ]
  }
}
