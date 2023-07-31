import radius as radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-terraform-helloworld'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-terraform-helloworld-env'
    }
    recipes: {
      'Applications.Link/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/empty-recipe.zip'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-terraform-helloworld'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-resources-terraform-helloworld-app'
      }
    ]
  }
}

resource webapp 'Applications.Link/extenders@2022-03-15-privatepreview' = {
  name: 'corerp-resources-terraform-helloworld'
  properties: {
    application: app.id
    environment: env.id
  }
}
