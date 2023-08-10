import radius as radius

@description('The base name of the test, used to qualify resources and namespaces. eg: corerp-resources-terraform-helloworld')
param basename string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: basename
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: '${basename}-env'
    }
    recipes: { // No recipes!
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

resource extender 'Applications.Link/extenders@2022-03-15-privatepreview' = {
  name: basename
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: 'not found!'
    }
  }
}
