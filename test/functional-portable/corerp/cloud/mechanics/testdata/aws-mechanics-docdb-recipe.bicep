extension radius

@description('Identifier and alias of the DocumentDB cluster the recipe creates.')
param clusterIdentifier string

@description('DB subnet group for the cluster.')
param subnetGroupName string

@description('Master user password of the cluster.')
@secure()
param masterUserPassword string

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
  name: 'corerp-mechanics-aws-docdb-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-mechanics-aws-docdb-env'
    }
    providers: {
      aws: {
        scope: '/planes/aws/aws/accounts/${awsAccountId}/regions/${awsRegion}'
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        docdb: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/extenders-aws-docdb-cluster-recipe:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-mechanics-aws-docdb-app'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-mechanics-aws-docdb-app'
      }
    ]
  }
}

resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'corerp-mechanics-aws-docdb'
  properties: {
    environment: env.id
    application: app.id
    recipe: {
      name: 'docdb'
      parameters: {
        clusterIdentifier: clusterIdentifier
        subnetGroupName: subnetGroupName
        masterUserPassword: masterUserPassword
        creationTimestamp: creationTimestamp
      }
    }
  }
}
