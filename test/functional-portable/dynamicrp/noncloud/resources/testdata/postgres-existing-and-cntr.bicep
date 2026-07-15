extension radius
extension testresources

@description('The ID of the shared environment that owns the existing postgres resource.')
param environment string

resource udtapp 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'dynamicrp-postgres-existing'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource udtcntr 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'postgres-cntr'
  location: 'global'
  properties: {
    application: udtapp.id
    environment: environment
    containers: {
      'postgres-cntr': {
        image: 'ghcr.io/radius-project/samples/demo:latest'
        ports: {
          web: {
            containerPort: 3000
          }
        }
        env: {
          CONNECTION_POSTGRES_HOST: {
            value: udtpgexisting.properties.host
          }
          CONNECTION_POSTGRES_PORT: {
            value: string(udtpgexisting.properties.port)
          }
          CONNECTION_POSTGRES_USERNAME: {
            value: udtpgexisting.properties.username
          }
          CONNECTION_POSTGRES_DATABASE: {
            value: udtpgexisting.properties.database
          }
          CONNECTION_POSTGRES_PASSWORD: {
            value: udtpgexisting.properties.password
          }
        }
      }
    }
  }
}

// Reference the env-scoped postgres resource provisioned by
// postgres-env-scoped-resource.bicep using the 'existing' keyword.
resource udtpgexisting 'Test.Resources/postgres@2025-01-01-preview' existing = {
  name: 'existing-postgres'
}
