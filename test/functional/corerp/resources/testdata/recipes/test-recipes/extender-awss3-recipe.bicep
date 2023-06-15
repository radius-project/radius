// import aws as aws 

import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: 'default'
}


// param context object

// param bucketName string = 'bucket${context.resource.id}'

// param bucketName string = 'test213124332423498'

// resource s3 'AWS.S3/Bucket@default' = {
//   alias: bucketName
//   properties: {
//     BucketName: bucketName
//   }
// }

// output result object = {
//   values: {
//     bucketName: s3.properties.BucketName
//   }
// }

resource svc 'core/Service@v1' = {
  metadata: {
    name: 'redis-${uniqueString('hello')}'
  }
  spec: {
    type: 'ClusterIP'
    selector: {
      app: 'redis'
      resource: 'hi'
    }
    ports: [
      {
        port: 6379
      }
    ]
  }
}
