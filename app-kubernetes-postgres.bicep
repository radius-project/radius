extension radius

@description('Username for Postgres db.')
param username string

@description('Password for Postgres db.')
@secure()
param password string

@description('GitLab Personal Access Token for accessing private modules')
@secure()
param gitlabPAT string

@description('Local Registry Server Token')
@secure()
param localRegistryToken string = 'test-token-123' // Default token for local testing

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
        authentication: {
          git: {
            pat: {
              'gitlab.com': {
                secret: gitlabSecrets.id
              }
            }
          }
        }
        registry: {
          mirror: 'https://dsl-qty-white-visited.trycloudflare.com'
          authentication: {
            token: {
              secret: localRegistryTokenSecret.id
            }
          }
        }
        version: {
          version: '1.7.0'
          releasesApiBaseUrl: 'http://host.docker.internal:8081/repository/terraform-releases'
          tls: {
            skipVerify: true
          }
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
          templatePath: 'git::https://gitlab.com/ytimocin-group/ytimocin-project.git//terraform-modules/postgres-kubernetes?ref=postgres-kubernetes/v1.0.0'
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

resource gitlabSecrets 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'gitlab-secrets'
  properties: {
    resource: 'app-postgres-env/gitlab-secrets'
    type: 'generic'
    data: {
      pat: {
        value: gitlabPAT
      }
      username: {
        value: 'oauth2' // GitLab supports oauth2 as username with PAT
      }
    }
  }
}

// Local registry token secret - Updated to use token authentication
resource localRegistryTokenSecret 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'local-registry-token-secret'
  properties: {
    resource: 'app-postgres-env/local-registry-token-secret'
    type: 'generic'
    data: {
      token: {
        value: localRegistryToken
      }
    }
  }
}
