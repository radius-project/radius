import radius as radius

param registry string 

param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'linkrp-resources-extender-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'linkrp-resources-extender-recipe-env' 
    }
    recipes: {
      'Applications.Link/extenders':{
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
  name: 'linkrp-resources-extender-recipe'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'linkrp-resources-extender-recipe-app'
      }
    ]
  }
}

resource extender 'Applications.Link/extenders@2022-03-15-privatepreview' = {
  name: 'extender-recipe-old'
  properties: {
    environment: env.id
    application: app.id
  }
}
