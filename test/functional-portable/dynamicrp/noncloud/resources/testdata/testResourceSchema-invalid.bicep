extension testresources
extension radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'udt-schemavalidation-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'udt-schemavalidation-env'
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'udt-schemavalidation-app'
  location: location
  properties: {
    environment: env.id
  }
}

// This resource should fail schema validation due to type mismatches
resource testResourceSchema 'Test.Resources/testResourceSchema@2023-10-01-preview'  = {
  name: 'udt-schemavalidation'
  location: location
  properties: {
    application: app.id
    environment: env.id
    validationData: 123
    }
}
