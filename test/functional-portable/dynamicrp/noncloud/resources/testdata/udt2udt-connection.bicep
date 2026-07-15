extension radius
extension testresources
param registry string

param version string

@description('Specifies the port the container listens on.')
param port int = 8080

resource recipepack 'Radius.Core/recipePacks@2025-08-01-preview' = {
  name: 'udt2udt-recipe-pack'
  location: 'global'
  properties: {
    recipes: {
      'Test.Resources/userTypeAlpha': {
        kind: 'bicep'
        source: '${registry}/test/testrecipes/test-bicep-recipes/dynamicrp_recipe:${version}'
        parameters: {
          port: port
        }
      }
    }
  }
}

resource udttoudtenv 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'udttoudtenv'
  location: 'global'
  properties: {
    recipePacks: [
      recipepack.id
    ]
    providers: {
      kubernetes: {
        namespace: 'dynamicrp-udt2udt'
      }
    }
  }
}

resource udttoudtapp 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'udttoudtapp'
  location: 'global'
  properties: {
    environment: udttoudtenv.id
  }
}

resource udttoudtparent 'Test.Resources/userTypeAlpha@2023-10-01-preview' = {
  name: 'udttoudtparent'
  properties: {
    environment: udttoudtenv.id
    application: udttoudtapp.id
    connections: {
      externalresource: {
        source: udttoudtchild.id
      }
    }
  }
}

resource udttoudtchild 'Test.Resources/externalResource@2023-10-01-preview' = {
  name: 'udttoudtchild'
  location: 'global'
  properties: {
    environment: udttoudtenv.id
    application: udttoudtapp.id
    configMap: '{"app1.sample.properties":"property1=value1\\nproperty2=value2","app2.sample.properties":"property3=value3\\nproperty4=value4"}'
  }
}
