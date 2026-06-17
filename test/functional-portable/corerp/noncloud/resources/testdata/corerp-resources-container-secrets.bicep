extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-container-secrets'
  location: location
  properties: {
    environment: environment
  }
}

resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'cntr-cntr-secrets'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      cntrcntrsecrets: {
        image: magpieimage
        env: {
          DB_USER: { value: 'DB_USER' }
          DB_PASSWORD: {
            valueFrom: {
              secretKeyRef: {
                secretName: saltysecret.name
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
}

resource saltysecret 'Radius.Security/secrets@2025-08-01-preview' = {
  name: 'saltysecret'
  location: location
  properties: {
    environment: environment
    application: app.id
    data: {
      DB_PASSWORD: {
        value: 'password'
      }
    }
  }
}
