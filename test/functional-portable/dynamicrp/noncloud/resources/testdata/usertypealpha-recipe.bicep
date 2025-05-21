extension testresources
extension radius

param registry string

param version string

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the port the container listens on.')
param port int = 8080

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'usertypealpha-recipe-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'usertypealpha-recipe-env'
    }
    recipes: {
      'Test.Resources/userTypeAlpha': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_recipe:${version}'
          parameters: {
            port: port
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'usertypealpha-recipe-app'
  location: location
  properties: {
    environment: env.id
  }
}

resource usertypealphacntr 'Applications.Core/containers@2023-10-01-preview' = {
    name: 'usertypealphacntr'
    properties: {
      application: app.id
      container: {
        image: 'ghcr.io/radius-project/mirror/debian:latest'
        command: ['/bin/sh']
        args: ['-c', 'while true; do echo hello; sleep 10;done']
        env: {
          USERTYPEALPHA_PORT: {
            value: string(usertypealpha.properties.status.binding.port)
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
    environment: env.id
  }
}
