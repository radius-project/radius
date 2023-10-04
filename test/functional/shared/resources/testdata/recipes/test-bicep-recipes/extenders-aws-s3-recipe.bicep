import aws as aws 

param creationTimestamp string
param bucketName string

resource s3Bucket 'AWS.S3/Bucket@default' = {
  alias: bucketName
  properties: {
    BucketName: bucketName
    Tags: [
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}
