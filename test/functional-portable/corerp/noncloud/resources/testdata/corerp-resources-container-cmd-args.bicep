extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-container-cmd-args'
  location: location
  properties: {
    environment: environment
  }
}

resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'ctnr-cmd-args'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      ctnrcmdargs: {
        image: 'ghcr.io/radius-project/mirror/debian:latest'
        command: ['/bin/sh']
        args: ['-c', 'while true; do echo hello; sleep 10;done']
      }
    }
  }
}
