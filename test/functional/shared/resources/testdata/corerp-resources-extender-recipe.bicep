import radius as radius

param registry string 

param version string

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
          templatePath: '${registry}/test/functional/shared/recipes/extender-recipe:${version}' 
          parameters: {
            containerImage: '${registry}/magpiego:${version}'
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
