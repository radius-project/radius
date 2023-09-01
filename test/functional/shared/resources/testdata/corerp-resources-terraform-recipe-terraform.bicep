import radius as radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('The Terraform recipe module to test. eg: kubernetes-redis')
param moduleName string

@description('The base name of the test, used to qualify resources and namespaces. eg: corerp-resources-terraform-helloworld')
param basename string

@description('The recipe name used to register the recipe. eg: default')
param environmentRecipeName string = 'default'

@description('The recipe name used to invoke the recipe. eg: default')
param resourceRecipeName string = 'default'

@description('The environment parameters to pass to the recipe. eg: {"message": "Hello World"}')
param environmentParameters object = {}

@description('The resource parameters to pass to the recipe. eg: {"name": "hello-world"}')
param resourceParameters object = {}

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
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
          templateKind: 'terraform'
          templatePath: '${moduleServer}/${moduleName}.zip'
          parameters: environmentParameters
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
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

resource webapp 'Applications.Core/extenders@2022-03-15-privatepreview' = {
  name: basename
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: resourceRecipeName
      parameters: resourceParameters
    }
  }
}
