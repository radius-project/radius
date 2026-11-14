extension testresources

@description('The ID of the environment to deploy into.')
param environment string

@description('The name of the ConfigMap the recipe creates.')
param physicalName string

resource configMap 'Test.Resources/renamableConfigMaps@2025-01-01-preview' = {
  name: 'renamableconfigmap'
  location: 'global'
  properties: {
    environment: environment
    physicalName: physicalName
  }
}
