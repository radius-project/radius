extension radius

@description('The URL of the server hosting test Terraform modules.')
param moduleServer string

param awsAccountId string
param awsRegion string
param bucketName string
param creationTimestamp string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-aws-s3-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-terraform-aws-s3-env'
    }
    providers: {
      aws: {
        scope: '/planes/aws/aws/accounts/${awsAccountId}/regions/${awsRegion}'
      }
    }
    recipes: {
      'Applications.Core/extenders': {
        default: {
          templateKind: 'terraform'
          templatePath: '${moduleServer}/aws-s3-bucket.zip'
          parameters: {
            bucket_name: bucketName
            creation_timestamp: creationTimestamp
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-aws-s3-app'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-resources-terraform-aws-s3-app'
      }
    ]
  }
}

resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: 'corerp-resources-terraform-aws-s3'
  properties: {
    environment: env.id
    application: app.id
  }
}
