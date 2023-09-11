import radius as radius

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-extender-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-extender-recipe-env' 
    }
    recipes: {
      'Applications.Core/extenders':{
        default: {
          templateKind: 'bicep'
          templatePath: 'shruku.azurecr.io/recipes/extender-invalid-test:1.0' 
          parameters: {
            containerImage: 'qwerty'
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-extender-recipe'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-extender-recipe-app'
      }
    ]
  }
}

resource extender 'Applications.Core/extenders@2022-03-15-privatepreview' = {
  name: 'extender-recipe'
  properties: {
    environment: env.id
    application: app.id
  }
}
