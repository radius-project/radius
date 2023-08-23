import radius as radius

param location string = resourceGroup().location

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Name of the Radius Application.')
param appName string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-terraform-azstorage-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-terraform-azstorage-env'
    }
    providers: {
      azure: {
        scope: resourceGroup().id
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/azure-storage.zip'
          parameters: {
            name: 'blob${uniqueString(resourceGroup().id)}'
            resource_group_name: resourceGroup().name
            location: location
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: appName
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: appName
      }
    ]
  }
}

resource webapp 'Applications.Core/extenders@2022-03-15-privatepreview' = {
  name: 'corerp-resources-terraform-azstorage'
  properties: {
    application: app.id
    environment: env.id
  }
}
