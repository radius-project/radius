extension radius

@secure()
param registryToken string

@description('Registry CA Certificate')
@secure()
param registryCACert string = ''

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'app-kubernetes-redis-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'app-kubernetes-redis-env'
    }
    recipeConfig: {
      terraform: {
        authentication: {
          git: {
            pat: {
              'devcloud.ubs.net': {
                secret: tokenSecretStore.id
              }
            }
          }
        }
        registry: {
          mirror: 'https://iac.devcloud.ubs.net/terraform/providers/mirror/'
          authentication: {
            token: {
              secret: registryTokenSecret.id
            }
          }
          tls: {
            caCertificate: {
              source: registryTLSCerts.id
              key: 'server.crt'
            }
          }
        }
        version: {
          releasesArchiveUrl: 'https://it4it-nexus-tp-repo.swissbank.com/repository/proxy-bin-crossplatform-hashicorp-raw-oss-consul/terraform/1.9.6/terraform_1.9.6_linux_amd64.zip'
          tls: {
            skipVerify: true
          }
        }
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: 'git::https://devcloud.ubs.net/ubs/ts/gcto/cpe/infra-as-code/iac/gitlab-central-registry/low-code/radius-recipes.git//recipes/kubernetes/redis'
          tls: {
            skipVerify: true
          }
        }
      }
    }
  }
}

resource tokenSecretStore 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'redis-terraform-git-token-store'
  properties: {
    resource: 'app-kubernetes-redis-env/redis-terraform-git-token-store'
    type: 'generic'
    data: {
      pat: {
        value: registryToken
      }
    }
  }
}

resource registryTokenSecret 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'redis-terraform-registry-token-store'
  properties: {
    resource: 'app-kubernetes-redis-env/redis-terraform-registry-token-store'
    type: 'generic'
    data: {
      token: {
        value: registryToken
      }
    }
  }
}

resource registryTLSCerts 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'redis-registry-tls-certs'
  properties: {
    resource: 'app-kubernetes-redis-env/redis-registry-tls-certs'
    type: 'generic'
    data: {
      'server.crt': {
        value: registryCACert
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'app-kubernetes-redis-app'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'app-kubernetes-redis-app'
      }
    ]
  }
}

resource redis 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'app-kubernetes-redis'
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: 'default'
      parameters: {
        redis_cache_name: 'app-kubernetes-redis'
      }
    }
  }
}
