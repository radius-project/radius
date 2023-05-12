import radius as radius

param scope string = resourceGroup().id

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'tf-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'tf-recipe-env' 
    }
    providers: {
      azure: {
        scope: scope
      }
    }
    recipes: {
      'Applications.Link/redisCaches':{
        default: {
          templateKind: 'terraform'
          templatePath: 'Azure/cosmosdb/azurerm' 
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'tf-recipe-app'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'tf-recipe-app'
      }
    ]
  }
}

resource redis 'Applications.Link/redisCaches@2022-03-15-privatepreview' = {
  name: 'tf-recipe'
  location: 'global'
  properties: {
    environment: env.id
    application: app.id
  }
}
