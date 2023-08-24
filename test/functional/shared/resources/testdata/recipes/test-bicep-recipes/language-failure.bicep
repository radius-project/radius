import radius as radius

param context object

var basename = context.resource.name

// This is not a realistic user scenario (creating a Radius resource in a recipe). We're
// doing things this way to test a bicep language failure without using cloud resources.
resource extender 'Applications.Core/extenders@2022-03-15-privatepreview' = {
  name: '${basename}-failure'
  properties: {
    application: context.application.id
    environment: context.environment.id
    resourceProvisioning: 'manual'
    #disable-next-line BCP234
    message: substring('abcd', 10, 2929999) // YOLO
  }
}
