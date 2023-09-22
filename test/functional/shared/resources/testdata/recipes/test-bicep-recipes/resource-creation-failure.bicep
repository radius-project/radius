import radius as radius

param context object

var basename = context.resource.name

// This is not a realistic user scenario (creating a Radius resource in a recipe). We're
// doing things this way to test a provisioning failure without using cloud resources.
resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: '${basename}-failure'
  properties: {
    application: 'not an id, just deal with it'
    environment: context.environment.id
    resourceProvisioning: 'manual'
    message: 'hello from recipe resource'
  }
}
