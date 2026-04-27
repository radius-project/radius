extension radius

@description('Name of the Radius Application.')
param appName string

resource bicepConfig 'Radius.Core/bicepConfigs@2025-08-01-preview' = {
  name: 'test-bicep-config'
  location: 'global'
  properties: {
    // No private registry auth configured; validates CRUD and environment reference.
    // Private registry auth tests require cloud infrastructure and belong in the cloud test suite.
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
