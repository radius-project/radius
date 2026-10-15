extension radius

@description('Recipe context supplied by Radius for the owning resource.')
param context object

// Use a same-scope Radius child to distinguish recipe ownership from outer template ownership.
resource application 'Radius.Core/applications@2025-08-01-preview' = {
  name: '${context.resource.name}-recipe-child'
  location: 'global'
  properties: {
    environment: context.environment.id
  }
}
