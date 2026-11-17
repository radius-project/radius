extension radius

@description('Name and alias of the MSK Serverless cluster the recipe creates.')
param clusterName string

@description('Comma-separated subnets for the cluster, in at least two Availability Zones.')
param subnetIds string

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
  name: 'corerp-mechanics-aws-msk-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-mechanics-aws-msk-env'
    }
    providers: {
      aws: {
        scope: '/planes/aws/aws/accounts/${awsAccountId}/regions/${awsRegion}'
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        msk: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/extenders-aws-msk-serverless-recipe:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-mechanics-aws-msk-app'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-mechanics-aws-msk-app'
      }
    ]
  }
}

resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'corerp-mechanics-aws-msk'
  properties: {
    environment: env.id
    application: app.id
    recipe: {
      name: 'msk'
      parameters: {
        clusterName: clusterName
        subnetIds: split(subnetIds, ',')
        creationTimestamp: creationTimestamp
      }
    }
  }
}
