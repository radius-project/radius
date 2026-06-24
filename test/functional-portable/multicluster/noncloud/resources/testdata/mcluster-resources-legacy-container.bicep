extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'mcluster-resources-legacy-container'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'mcluster-resources-legacy-container-app'
      }
    ]
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'mcluster-legacy-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: 'ghcr.io/radius-project/mirror/debian:latest'
      command: ['/bin/sh']
      args: ['-c', 'while true; do echo hello; sleep 10; done']
    }
    connections: {}
  }
}
