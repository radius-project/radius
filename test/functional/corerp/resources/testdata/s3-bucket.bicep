import aws as aws

param bucketName string

resource bucket 'AWS.S3/Bucket@default' = {
  alias: bucketName
  properties: {
    BucketName: bucketName
    AccessControl: 'Private'
  }
}
