
extension radius

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-container-secrets'
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-container-secrets'
      }
    ]
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'cntr-cntr-secrets'
  properties: {
      application: app.id
      container: {
        image: magpieimage
        env: {
          DB_USER: { value: 'DB_USER' }
          DB_PASSWORD: {
            valueFrom: {
              secretRef: {
                source: saltysecret.id
                key: 'DB_PASSWORD'
              }
            }
          }
        }
        ports: {
          web: {
            containerPort: 5000
          }
        }
      }
    }
  }

  resource saltysecret 'Applications.Core/secretStores@2023-10-01-preview' = {
    name: 'saltysecret'
    properties: {
      application: app.id
      data: {
        DB_PASSWORD: {
          value: 'password'
        }
      }
    }
  }
