import radius as radius
import kubernetes as kubernetes {
  namespace: 'corerp-mechanics-k8s-extensibility'
  kubeConfig: ''
}

// TODO: https://github.com/Azure/bicep/issues/7553 
// this bug blocks the use of 'existing' for Kubernetes resources. Once that's fixed we can
// restore these to the test. 

// resource redisService 'core/Service@v1' existing = {
//   metadata: {
//     name: 'redis-master'
//   }
// }

// resource redisSecret 'core/Secret@v1' existing = {
//   metadata: {
//     name: 'redis'
//   }
// }

resource secret 'core/Secret@v1' = {
  metadata: {
    name: 'redis-conn'
    labels: {
      format: 'k8s-extension'
    }
  }

  stringData: {
    connectionString: 'redis-master.corerp-mechanics-k8s-extensibility.svc.cluster.local,password=awesome'
    // TODO: https://github.com/Azure/bicep/issues/7553 
    //connectionString: '${redisService.metadata.name}.${redisService.metadata.namespace}.svc.cluster.local,password=${base64ToString(redisSecret.data.redisPassword)}'
  }
}

param environment string

// Our test framework wants an app, but we don't really want any radius resources for this test.
resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-k8s-extensibility'
  location: 'global'
  properties: {
    environment: environment
  }
}
