provider radius

@description('The OCI registry for test Bicep recipes.')
param registry string
@description('The OCI tag for test Bicep recipes.')
param version string
@description('Specifies the location for resources.')
param location string = 'global'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-extender-recipe-env'
  location: location
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
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/extender-recipe:${version}' 
          parameters: {
            containerImage: '${registry}/magpiego:${version}'
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-extender-recipe'
  location: location
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

resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'extender-recipe'
  properties: {
    environment: env.id
    application: app.id
  }
}
