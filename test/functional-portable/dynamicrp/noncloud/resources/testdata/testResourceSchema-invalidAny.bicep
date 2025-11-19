extension testresources
extension radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'udt-unconstrained-validation-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'udt-unconstrained-validation-env'
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'udt-unconstrained-validation-app'
  location: location
  properties: {
    environment: env.id
  }
}

// This resource should fail schema validation due to using type: any on a direct property (not additionalProperties under platformOptions)
resource testInvalidUnconstrained 'Test.Resources/testInvalidUnconstrainedSchema@2023-10-01-preview' = {
  name: 'udt-invalid-unconstrained'
  location: location
  properties: {
    application: app.id
    environment: env.id
    invalidField: {
      canBeAnything: 'value'
    }
  }
}
