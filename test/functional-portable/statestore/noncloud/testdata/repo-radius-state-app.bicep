extension radius

@description('The Radius environment resource ID for the application.')
param environment string

@description('A marker written by the container to prove that an update was deployed.')
param deploymentPhase string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'repo-radius-state-e2e'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'repo-radius-state-container'
  location: 'global'
  properties: {
    application: app.id
    environment: environment
    containers: {
      main: {
        image: 'ghcr.io/radius-project/mirror/debian:latest'
        command: ['/bin/sh']
        args: ['-c', 'while true; do echo ${deploymentPhase}; sleep 10; done']
      }
    }
    connections: {}
  }
}
