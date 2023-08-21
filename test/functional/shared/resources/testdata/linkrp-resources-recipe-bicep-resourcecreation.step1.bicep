import radius as radius

@description('The base name of the test, used to qualify resources and namespaces. eg: linkrp-resources-terraform-helloworld')
param basename string
@description('The recipe name used to register the recipe. eg: default')
param environmentRecipeName string = 'default'

resource env 'Applications.Core/environments@2022-03-15-privatepreview' existing = {
  name: basename
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' existing = {
  name: basename
}

resource extender 'Applications.Link/extenders@2022-03-15-privatepreview' = {
  name: basename
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: environmentRecipeName
    }
  }
}
