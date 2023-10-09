import radius as radius

param registry string 

param version string

param magpieimage string 

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'dsrp-resources-simenv-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'dsrp-resources-simenv-recipe-env'
    }
    recipes: {
      'Applications.Datastores/mongoDatabases':{
        'mongodb-recipe-kubernetes': {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/mongodb-recipe-kubernetes:${version}' 
        }
      }
    }
    simulated: true
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'dsrp-resources-simenv-recipe'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'dsrp-resources-simenv-recipe-app'
      }
    ]
  }
}

resource webapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'mongodb-app-ctnr-simenv'
  location: 'global'
  properties: {
    application: app.id
    connections: {
      mongodb: {
        source: recipedb.id
      }
    }
    container: {
      image: magpieimage
      env: {
        DBCONNECTION: recipedb.connectionString()
      }
      readinessProbe:{
        kind:'httpGet'
        containerPort:3000
        path: '/healthz'
      }
    }
  }
}

resource recipedb 'Applications.Datastores/mongoDatabases@2023-10-01-preview' = {
  name: 'mongodb-db-simenv'
  location: 'global'
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: 'mongodb-recipe-kubernetes'
    }
  }
}
