extension radius
extension testresources
param registry string

param version string

@description('PostgreSQL password')
@secure()
param password string = newGuid()

resource udtconnenv 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'dynamicrp-postgres-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'dynamicrp-postgres-env'
    }
    recipes: {
      'Test.Resources/postgres': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_postgress_recipe:${version}'
        }
      }
    }
  }
}

resource udtapp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'dynamicrp-cntr2udt'
  location: 'global'
  properties: {
    environment: udtconnenv.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'dynamicrp-cntr2udt'
      }
    ]
  }
}


resource udtcntr 'Applications.Core/containers@2023-10-01-preview' = {
    name: 'udtcntr'
    properties: {
      application: udtapp.id
      container: {
        image: 'ghcr.io/radius-project/samples/demo:latest'
        ports: {
          web: {
            containerPort: 3000
          }
        }

    
  
    }
      connections: {
        postgres: {
          source: udtconnpg.id
        }
      }
  
    }
}


resource udtconnpg 'Test.Resources/postgres@2023-10-01-preview' = {
  name: 'existing-postgres'
  location: 'global'
  properties: {
    environment: udtconnenv.id
    password: password
    port: '5432'
  }
}
