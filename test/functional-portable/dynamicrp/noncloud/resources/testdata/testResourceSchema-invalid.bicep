extension testresources
extension radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'udt-schemavalidation-env'
  location: location
  properties: {
    providers: {
      kubernetes: {
        namespace: 'udt-schemavalidation-app'
      }
    }
  }
}

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'udt-schemavalidation-app'
  location: location
  properties: {
    environment: env.id
  }
}

// This resource should fail schema validation due to type mismatches
resource testResourceSchema 'Test.Resources/testResourceSchema@2023-10-01-preview' = {
  name: 'udt-schemavalidation'
  location: location
  properties: {
    application: app.id
    environment: env.id
    validationData: 123
  }
}
