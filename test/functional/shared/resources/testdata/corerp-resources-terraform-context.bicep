import radius as radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-context'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-terraform-context-env'
    }
    providers: {
      azure: {
        // This is the fake scope for testing purposes.
        scope: '/subscriptions/00000000-0000-0000-0000-100000000000/resourceGroups/rg-terraform-context'
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/k8ssecret-context.zip'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-context'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-resources-terraform-context-app'
      }
    ]
  }
}

resource webapp 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-context'
  properties: {
    application: app.id
    environment: env.id
  }
}
