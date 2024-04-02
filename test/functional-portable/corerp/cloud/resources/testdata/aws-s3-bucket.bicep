import aws as aws

param creationTimestamp string
param bucketName string

resource bucket 'AWS.S3/Bucket@default' = {
  alias: bucketName
  properties: {
    BucketName: bucketName
    Tags: [
      {
        Key: 'testKey'
        Value: 'testValue'
      }
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}
