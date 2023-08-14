import radius as rad

param bucketName string
param awsAccountId string
param awsRegion string
param registry string 
param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'linkrp-resources-extenders-aws-s3-recipe-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'linkrp-resources-extenders-aws-s3-recipe-env'
    }
    providers: {
      aws: {
        scope: '/planes/aws/aws/accounts/${awsAccountId}/regions/${awsRegion}'
      }
    }
    recipes: {
      'Applications.Link/extenders': {
        s3: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/extenders-aws-s3-recipe:${version}' 
          parameters: {
            bucketName: bucketName
          }
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'linkrp-resources-extenders-aws-s3-recipe-app'
  location: 'global'
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'linkrp-resources-extenders-aws-s3-recipe-app'
      }
    ]
  }
}

resource extender 'Applications.Link/extenders@2022-03-15-privatepreview' = {
  name: 'linkrp-resources-extenders-aws-s3-recipe'
  properties: {
    environment: env.id
    application: app.id
    recipe: {
      name: 's3'
      parameters: {
        bucketName: bucketName
      }
    }
  }
}
