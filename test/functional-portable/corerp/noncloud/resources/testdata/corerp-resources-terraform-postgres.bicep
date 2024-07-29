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
      terraform: {
        providers: {
          postgresql: [ {
              alias: 'pgdb-test'
              username: userName
              password: password
              sslmode: 'disable'
              secrets: {
                host: {
                  source: pgshostsecret.id
                  key: 'host'
                }
              }
            } ]
        }
      }
      env: {
        PGPORT: '5432'
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
  name: 'pgs-resources-terraform-pgsapp'
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

resource pgshostsecret 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'pgs-hostsecret'
  properties: {
    resource: 'corerp-resources-terraform-pg-app/pgs-hostsecret'
    type: 'generic'
    data: {
      host: {
        value: 'postgres.corerp-resources-terraform-pg-app.svc.cluster.local'
      }
    }
  }
}
