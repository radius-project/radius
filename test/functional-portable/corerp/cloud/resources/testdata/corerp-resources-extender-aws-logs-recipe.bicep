extension radius

param logGroupName string
param creationTimestamp string
param awsAccountId string
param awsRegion string
param registry string 
param version string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-extenders-aws-logs-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-extenders-aws-logs-recipe-env'
    }
    providers: {
      aws: {
        scope: '/planes/aws/aws/accounts/${awsAccountId}/regions/${awsRegion}'
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        logs: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/extenders-aws-logs-recipe:${version}' 
          parameters: {
            logGroupName: logGroupName
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-extenders-aws-logs-recipe-app'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'corerp-resources-extenders-aws-logs-recipe-app'
      }
    ]
  }
}

resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'corerp-resources-extenders-aws-logs-recipe'
  properties: {
    environment: env.id
    application: app.id
    recipe: {
      name: 'logs'
      parameters: {
        creationTimestamp: creationTimestamp
        logGroupName: logGroupName
      }
    }
  }
}