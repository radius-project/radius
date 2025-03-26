extension radius

param registry string

param version string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'dsrp-resources-mongodb-recipe-and-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'dsrp-resources-mongodb-recipe-and-env'
    }
    recipes: {
      'Applications.Datastores/mongoDatabases': {
        mongokubernetes: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/mongodb-recipe-for-existing-resource:${version}'
        }
      }
    }
  }
}

resource mongodbEnvScoped 'Applications.Datastores/mongoDatabases@2023-10-01-preview' = {
  name: 'mongodb-db-existing'
  location: 'global'
  properties: {
    environment: env.id
    recipe: {
      name: 'mongokubernetes'
    }
  }
}

