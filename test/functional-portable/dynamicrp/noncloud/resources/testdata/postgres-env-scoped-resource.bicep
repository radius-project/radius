extension testresources
extension radius

@description('The ID of the shared environment to deploy the postgres resource into.')
param environment string

@description('PostgreSQL password')
@secure()
param password string = newGuid()

// Environment-scoped postgres resource provisioned by the custom postgres recipe.
// The recipe is registered via the custom recipe pack attached to the preview
// environment; the container recipe comes from the default recipe pack.
resource udtpg 'Test.Resources/postgres@2025-01-01-preview' = {
  name: 'existing-postgres'
  location: 'global'
  properties: {
    environment: environment
    password: password
  }
}
