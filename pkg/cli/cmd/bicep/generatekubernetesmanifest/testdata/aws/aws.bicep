extension radius
extension aws

param bucketName string = 'gkm-bucket'

resource bucket 'AWS.S3/Bucket@default' = {
  alias: bucketName
  properties: {
    BucketName: bucketName
  }
}
