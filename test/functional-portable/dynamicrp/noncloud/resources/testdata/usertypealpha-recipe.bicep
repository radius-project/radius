extension testresources
extension radius

@description('The ID of the environment to deploy into.')
param environment string

@description('Specifies the location for resources.')
param location string = 'global'

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'usertypealpha-recipe-app'
  location: location
  properties: {
    environment: environment
  }
}

resource usertypealphacntr 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'usertypealphacntr'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      usertypealphacntr: {
        image: 'ghcr.io/radius-project/mirror/debian:latest'
        command: ['/bin/sh']
        args: ['-c', 'while true; do echo hello; sleep 10;done']
        env: {
          USERTYPEALPHA_PORT: {
            value: string(usertypealpha.properties.port)
          }
        }
      }
    }
  }
}

resource usertypealpha 'Test.Resources/userTypeAlpha@2023-10-01-preview' = {
  name: 'usertypealphainstance'
  location: location
  properties: {
    application: app.id
    environment: environment
  }
}

resource usertypealphalatest 'Test.Resources/userTypeAlpha@2025-01-01-preview' = {
  name: 'usertypealphalatest'
  location: location
  properties: {
    application: app.id
    environment: environment
  }
}
