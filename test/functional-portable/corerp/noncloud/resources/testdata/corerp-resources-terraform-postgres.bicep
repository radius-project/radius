extension radius

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
      terraform: {
        providers: {
          postgresql: [
            {
              alias: 'pgdb-test'
              sslmode: 'disable'
              secrets: {
                username: {
                  source: pgsecretstore.id
                  key: 'username'
                }
                password: {
                  source: pgsecretstore.id
                  key: 'password'
                }
              }
            }
          ]
        }
      }
      env: {
        PGPORT: '5432'
      }
      envSecrets: {
        PGHOST: {
          source: pgsecretstore.id
          key: 'host'
        }
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        defaultpostgres: {
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
  name: 'corerp-resources-terraform-pgsapp'
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: 'defaultpostgres'
      parameters: {
        password: password
      }
    }
  }
}

resource pgsecretstore 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'pgs-secretstore'
  properties: {
    resource: 'corerp-resources-terraform-pg-app/pgs-secretstore'
    type: 'generic'
    data: {
      username: {
        value: userName
      }
      password: {
        value: password
      }
      host: {
        value: 'postgres.corerp-resources-terraform-pg-app.svc.cluster.local'
      }
    }
  }
}
