extension testresources
extension radius

param registry string

param version string

resource udtenv 'Applications.Core/environments@2023-10-01-preview' = {
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

resource udtpg 'Test.Resources/postgres@2023-10-01-preview' = {
  name: 'existing-postgres'
  location: 'global'
  properties: {
    environment: udtenv.id
  }
}
