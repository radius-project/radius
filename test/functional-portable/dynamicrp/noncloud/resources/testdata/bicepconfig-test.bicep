extension radius

@description('Name of the Radius Application.')
param appName string

resource bicepConfig 'Radius.Core/bicepConfigs@2025-08-01-preview' = {
  name: 'test-bicep-config'
  location: 'global'
  properties: {
    registryAuthentication: {
      authenticationMethod: 'BasicAuth'
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'bicepconfig-test-env'
  location: 'global'
  properties: {
    providers: {
      kubernetes: {
        namespace: 'bicepconfig-test-ns'
      }
    }
    bicepConfig: bicepConfig.id
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  location: 'global'
  properties: {
    environment: env.id
  }
}
