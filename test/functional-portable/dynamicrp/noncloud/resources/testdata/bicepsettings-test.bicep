extension radius

@description('Name of the Radius Application.')
param appName string

// SecretStore providing the username/password for BasicAuth registry access.
// The values are placeholders since this test exercises CRUD wiring and
// environment reference validation, not an actual private registry pull.
resource registrySecret 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'bicepsettings-test-secret'
  location: 'global'
  properties: {
    resource: 'bicepsettings-test-ns/bicepsettings-test-secret'
    type: 'generic'
    data: {
      username: { value: 'test-user' }
      password: { value: 'test-pass' }
    }
  }
}

resource bicepSettings 'Radius.Core/bicepSettings@2025-08-01-preview' = {
  name: 'test-bicep-config'
  location: 'global'
  properties: {
    registryAuthentications: {
      'corp.acr.example.io': {
        authenticationMethod: 'BasicAuth'
        basicAuthSecretId: registrySecret.id
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'bicepsettings-test-env'
  location: 'global'
  properties: {
    providers: {
      kubernetes: {
        namespace: 'bicepsettings-test-ns'
      }
    }
    bicepSettings: bicepSettings.id
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  location: 'global'
  properties: {
    environment: env.id
  }
}
