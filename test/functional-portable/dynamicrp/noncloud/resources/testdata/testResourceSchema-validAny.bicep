extension testresources
extension radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'udt-platformoptions-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'udt-platformoptions-env'
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
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
