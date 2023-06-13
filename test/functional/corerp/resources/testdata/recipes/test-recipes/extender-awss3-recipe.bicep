import aws as aws 

param context object

param bucketName string = 'bucket${context.resource.id}'

resource s3 'AWS.S3/Bucket@default' = {
  alias: bucketName
  properties: {
    BucketName: bucketName
  }
}

output result object = {
  values: {
    bucketName: s3.properties.BucketName
  }
}
