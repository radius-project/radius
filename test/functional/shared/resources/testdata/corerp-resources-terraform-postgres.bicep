import radius as radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

@description('Username for Postgres db.')
param userName string

@description('Password for Postgres db.')
@secure()
param password string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-pg-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-terraform-pg-env'
    }
    recipeConfig: {
      terraform:{
        providers:{
          postgresql:[{
            username: userName
            port: 5432 
            password: password
            sslmode: 'disable'
          }]
        }
      }
      env: {
          PGHOST: 'postgres.corerp-resources-terraform-pg-app.svc.cluster.local'
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/postgres.zip'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-pg-app'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-resources-terraform-pg-app'
      }
    ]
  }
}

resource pgsapp 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'pgs-resources-terraform-pgsapp'
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: 'default'
      parameters: {
         password: password
      }
    }
  }
}
	