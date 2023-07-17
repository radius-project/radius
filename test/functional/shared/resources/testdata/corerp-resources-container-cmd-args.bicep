import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-container-cmd-args'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-cmd-args-app'
      }
    ]
  }
}

resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'ctnr-cmd-args'
  location: location
  properties: {
    application: app.id
    container: {
      image: 'debian'
      command: ['/bin/sh']
      args: ['-c', 'while true; do echo hello; sleep 10;done']
    }
    connections: {}
  }
}
