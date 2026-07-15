extension testresources
extension radius

param registry string

param version string

@description('Specifies the location for resources.')
param location string = 'global'

@secure()
param password string
@secure()
param apiKey string
@secure()
param credentialSecret string
@secure()
param connectionConfigUrl string
@secure()
param connectionConfigToken string

resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'udt-sensitive-recipe-pack'
  location: location
  properties: {
    recipes: {
      'Test.Resources/sensitiveResource': {
        kind: 'bicep'
        source: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_sensitive_recipe:${version}'
      }
    }
  }
}

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'udt-sensitive-env'
  location: location
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'udt-sensitive-app'
      }
    }
  }
}

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'udt-sensitive-app'
  location: location
  properties: {
    environment: env.id
  }
}

resource sensitiveRes 'Test.Resources/sensitiveResource@2023-10-01-preview' = {
  name: 'udt-sensitive-instance'
  location: location
  properties: {
    application: app.id
    environment: env.id
    username: 'admin'
    password: password
    apiKey: apiKey
    credentials: {
      host: 'db.example.com'
      secret: credentialSecret
    }
    connectionConfig: {
      url: connectionConfigUrl
      token: connectionConfigToken
    }
  }
}
