extension testresources
extension radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'udt-platformoptions-env'
  location: location
  properties: {
    providers: {
      kubernetes: {
        namespace: 'udt-platformoptions-app'
      }
    }
  }
}

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'udt-platformoptions-app'
  location: location
  properties: {
    environment: env.id
  }
}

// This resource should PASS validation - platformOptions can have unconstrained additionalProperties
resource testValidPlatformOptions 'Test.Resources/testValidPlatformOptionsSchema@2023-10-01-preview' = {
  name: 'udt-valid-platformoptions'
  location: location
  properties: {
    application: app.id
    environment: env.id
    platformOptions: {
      customKey1: 'value1'
      customKey2: 'value2'
      nested: {
        key: 'value'
      }
    }
  }
}
