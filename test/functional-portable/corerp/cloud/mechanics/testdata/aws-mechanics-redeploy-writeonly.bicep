extension aws

@description('Name and alias of the bucket.')
param bucketName string

@description('Unix time the test started, used by the job that purges leaked test resources.')
param creationTimestamp string

// BucketNamespace is write-only and create-only: AWS never returns it and it cannot change after create.
resource bucket 'AWS.S3/Bucket@default' = {
  alias: bucketName
  properties: {
    BucketName: bucketName
    BucketNamespace: 'global'
    Tags: [
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}
