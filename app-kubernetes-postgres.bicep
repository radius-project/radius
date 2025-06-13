extension radius

@description('Username for Postgres db.')
param username string

@description('Password for Postgres db.')
@secure()
param password string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'app-postgres-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'app-postgres-env'
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
        version: {
          version: '1.7.0'
          releasesApiBaseUrl: 'http://localhost:8081/repository/terraform'
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
          templatePath: 'http://localhost:8081/repository/terraform-releases/modules/kubernetes/postgres/1.0.0/postgres.zip'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'app-postgres'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'app-postgres'
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
    resource: 'app-postgres/pgs-secretstore'
    type: 'generic'
    data: {
      username: {
        value: username
      }
      password: {
        value: password
      }
      host: {
        value: 'postgres.app-postgres.svc.cluster.local'
      }
    }
  }
}
