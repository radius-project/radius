import radius as radius

@description('The OCI registry for test Bicep recipes.')
param registry string
@description('The OCI tag for test Bicep recipes.')
param version string

@description('The base name of the test, used to qualify resources and namespaces. eg: corerp-resources-terraform-helloworld')
param basename string
@description('The recipe to test. eg: hello-world')
param recipe string
@description('The recipe name used to register the recipe. eg: default')
param environmentRecipeName string = 'default'
@description('The environment parameters to pass to the recipe. eg: {"message": "Hello World"}')
param environmentParameters object = {}

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: basename
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: '${basename}-env'
    }
    recipes: {
      'Applications.Core/extenders': {
        '${environmentRecipeName}': {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/${recipe}:${version}'
          parameters: environmentParameters
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: basename
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: '${basename}-app'
      }
    ]
  }
}

// This resources is intentionally NOT using a recipe. It's being created so we can reference
// it inside a recipe in the next step.
resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: '${basename}-existing'
  properties: {
    application: app.id
    environment: env.id
    resourceProvisioning: 'manual'
    message: 'hello from existing resource'
  }
}
