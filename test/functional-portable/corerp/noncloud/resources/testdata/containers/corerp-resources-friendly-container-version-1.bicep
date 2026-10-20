extension radius

param magpieimage string
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-container-versioning'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource webapp 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'friendly-ctnr'
  location: 'global'
  properties: {
    application: app.id
    environment: environment
    containers: {
      friendlyctnr: {
        image: magpieimage
        env: {
          DB_PASSWORD: {
            valueFrom: {
              secretKeyRef: {
                secretName: friendlysecret.name
                key: 'DB_PASSWORD'
              }
            }
          }
        }
      }
    }
  }
}

resource friendlysecret 'Radius.Security/secrets@2025-08-01-preview' = {
  name: 'friendly-secret'
  location: 'global'
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
