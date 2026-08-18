extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the Radius application name.')
param applicationName string = 'corerp-resources-container-secrets'

@description('Specifies the Radius container resource name.')
param containerName string = 'cntr-cntr-secrets'

@description('Specifies the Radius secret resource name.')
param secretName string = 'saltysecret'

@secure()
@description('Specifies the username stored in the secret.')
param secretUsername string

@secure()
@description('Specifies the base64-encoded password stored in the secret.')
param secretPassword string

@secure()
@description('Specifies the API key stored in the secret.')
param secretApiKey string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: applicationName
  location: location
  properties: {
    environment: environment
  }
}

resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: containerName
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      secretcheck: {
        initContainer: true
        image: magpieimage
        command: [
          '/bin/sh'
          '-c'
        ]
        args: [
          'test -n "$CONNECTION_CREDENTIALS_USERNAME" && test -n "$CONNECTION_CREDENTIALS_PASSWORD" && test -n "$CONNECTION_CREDENTIALS_APIKEY"'
        ]
      }
      app: {
        image: magpieimage
        env: {
          CONNECTION_CREDENTIALS_USERNAME: {
            value: 'explicit-user'
          }
        }
        ports: {
          web: {
            containerPort: 5000
          }
        }
      }
    }
    connections: {
      credentials: {
        source: saltysecret.id
      }
      disabledCredentials: {
        source: saltysecret.id
        disableDefaultEnvVars: true
      }
    }
  }
}

resource saltysecret 'Radius.Security/secrets@2025-08-01-preview' = {
  name: secretName
  location: location
  properties: {
    environment: environment
    application: app.id
    data: {
      username: {
        value: secretUsername
      }
      password: {
        encoding: 'base64'
        value: secretPassword
      }
      apiKey: {
        value: secretApiKey
      }
    }
  }
}
