import radius as radius

param context object

var basename = context.resource.name

// This is not a realistic user scenario (creating a Radius resource in a recipe). We're
// doing things this way to test the UCP functionality without using cloud resources.
resource extender 'Applications.Core/extenders@2022-03-15-privatepreview' = {
  name: '${basename}-created'
  properties: {
    application: context.application.id
    environment: context.environment.id
    resourceProvisioning: 'manual'
    message: 'hello from recipe resource'
  }
}

#disable-next-line no-unused-existing-resources
resource existing 'Applications.Core/extenders@2022-03-15-privatepreview' existing = {
  name: '${basename}-existing'
}

module mod '_resource-creation.bicep' = {
  name: '${basename}-module'
  params: {
    context: context
  }
}

output result object = {
  resources: [
    // We don't actually need to create this, just to make sure the recipe engine
    // processes it. 
    '/planes/kubernetes/local/namespaces/${context.runtime.kubernetes.namespace}/providers/core/Secret/${context.resource.name}'
  ]
}
