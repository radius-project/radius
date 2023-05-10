import radius as radius

param rg string = resourceGroup().name

param sub string = subscription().subscriptionId

param magpieimage string 

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-environment-recipes-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-environment-recipes-env'
    }
    providers: {
      azure: {
        scope: '/subscriptions/${sub}/resourceGroups/${rg}'
      }
    }
    recipes: {
      'Applications.Link/mongoDatabases':{
        mongodb: {
          templatePath: 'vishwaradius.azurecr.io/recipes/mongo-test:1.0'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-mongodb-recipe'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-mongodb-recipe-app'
      }
    ]
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'mongodb-recipe-app-ctnr'
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

resource recipedb 'Applications.Link/mongoDatabases@2022-03-15-privatepreview' = {
  name: 'mongo-recipe-db'
  location: 'global'
  properties: {
    application: app.id
    environment: env.id
    mode: 'recipe'
    recipe: {
      name: 'mongodb'
    }
  }
}
