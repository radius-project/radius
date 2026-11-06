extension radius

@description('Name and alias of the log group the recipe creates.')
param logGroupName string

@description('Alias and Name tag of the DHCP option set the recipe creates.')
param dhcpOptionsName string

@description('Unix time the test started, used by the job that purges leaked test resources.')
param creationTimestamp string

@description('AWS account the environment deploys to.')
param awsAccountId string

@description('AWS Region the environment deploys to.')
param awsRegion string

@description('Registry that holds the test recipes.')
param registry string

@description('Tag of the test recipes.')
param version string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-mechanics-aws-recipe-delete-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-mechanics-aws-recipe-delete-env'
    }
    providers: {
      aws: {
        scope: '/planes/aws/aws/accounts/${awsAccountId}/regions/${awsRegion}'
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        awsresources: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/extenders-aws-multiple-resources-recipe:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-mechanics-aws-recipe-delete-app'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-mechanics-aws-recipe-delete-app'
      }
    ]
  }
}

resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'corerp-mechanics-aws-recipe-delete'
  properties: {
    environment: env.id
    application: app.id
    recipe: {
      name: 'awsresources'
      parameters: {
        logGroupName: logGroupName
        dhcpOptionsName: dhcpOptionsName
        creationTimestamp: creationTimestamp
      }
    }
  }
}
