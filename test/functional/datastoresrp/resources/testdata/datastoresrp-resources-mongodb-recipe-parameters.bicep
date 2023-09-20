import radius as radius

param rg string = resourceGroup().name

param sub string = subscription().subscriptionId

param registry string 

param version string

param magpieimage string 

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'dsrp-resources-env-recipe-parameters-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'dsrp-resources-env-recipe-parameters-env'
    }
    providers: {
      azure: {
        scope: '/subscriptions/${sub}/resourceGroups/${rg}'
      }
    }
    recipes: {
      'Applications.Datastores/mongoDatabases' :{
        mongodb: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/mongodb-recipe-parameter:${version}'
          parameters: {
            documentdbName: 'acnt-operator-${rg}'
            mongodbName: 'mdb-operator-${rg}'
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'dsrp-resources-mongodb-recipe-parameters'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'dsrp-resources-mongodb-recipe-param-app'
      }
    ]
  }
}

resource webapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'mdb-param-ctnr'
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
  name: 'mdb-recipe-param-db'
  location: 'global'
  properties: {
    application: app.id
    environment: env.id
    recipe: {
      name: 'mongodb'
      parameters: {
        documentdbName: 'acnt-developer-${rg}'
      }
    }
  }
}
