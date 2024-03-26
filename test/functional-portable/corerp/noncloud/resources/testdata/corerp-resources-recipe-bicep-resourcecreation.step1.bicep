import radius as radius

@description('The base name of the test, used to qualify resources and namespaces. eg: corerp-resources-terraform-helloworld')
param basename string
@description('The recipe name used to register the recipe. eg: default')
param environmentRecipeName string = 'default'

resource env 'Applications.Core/environments@2023-10-01-preview' existing = {
  name: basename
}

resource app 'Applications.Core/applications@2023-10-01-preview' existing = {
  name: basename
}

resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: basename
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: environmentRecipeName
    }
  }
}
