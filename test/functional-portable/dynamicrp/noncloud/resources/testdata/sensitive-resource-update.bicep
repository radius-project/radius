extension testresources
extension radius

@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'udt-sensitive-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'udt-sensitive-env'
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'udt-sensitive-app'
  location: location
  properties: {
    environment: env.id
  }
}

resource sensitiveRes 'Test.Resources/sensitiveResource@2023-10-01-preview' = {
  name: 'udt-sensitive-instance'
  location: location
  properties: {
    application: app.id
    environment: env.id
    username: 'admin'
    password: 'updated-secret-password'
    apiKey: 'ak_updated_key_xyz'
    credentials: {
      host: 'db.example.com'
      secret: 'updated-nested-secret'
    }
  }
}
