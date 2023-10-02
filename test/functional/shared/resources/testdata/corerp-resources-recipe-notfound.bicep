import radius as radius

@description('The base name of the test, used to qualify resources and namespaces. eg: corerp-resources-terraform-helloworld')
param basename string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
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

resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: basename
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: 'not found!'
    }
  }
}
