extension radius
extension testresources
param registry string

param version string

@description('PostgreSQL password')
@secure()
param password string = newGuid()

resource udttoudtenv 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'udttoudtenv'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'udttoudtenv'
    }
    recipes: {
      'Test.Resources/postgres': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_postgress_recipe:${version}'
        }
      }
      'Test.Resources/udtParent': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/parent-udt:${version}'
        }
      }
    }
  }
}

resource udttoudtapp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'udttoudtapp'
  location: 'global'
  properties: {
    environment: udttoudtenv.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'udttoudtapp'
      }
    ]
  }
}


resource udtparent 'Test.Resources/udtParent@2023-10-01-preview' = {
    name: 'udtparent'
    properties: {
      environment: udttoudtenv.id
      password: password
      port: '5432'
      connections: {
        postgres: {
          source: udtchild.id
        }
      }
    }     
}


resource udtchild 'Test.Resources/postgres@2023-10-01-preview' = {
  name: 'udtchild'
  location: 'global'
  properties: {
    environment: udttoudtenv.id
    password: password
    port: '5432'
  }
}
