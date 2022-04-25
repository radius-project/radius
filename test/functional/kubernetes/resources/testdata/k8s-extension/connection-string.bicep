import kubernetes from kubernetes

resource redisService 'kubernetes.core/Service@v1' existing = {
  metadata: {
    name: 'redis-master'
    namespace: 'k8s-extension'
  }
}

resource redisSecret 'kubernetes.core/Secret@v1' existing = {
  metadata: {
    name: 'redis'
    namespace: 'k8s-extension'
  }
}

resource secret 'kubernetes.core/Secret@v1' = {
  metadata: {
    name: 'redis-conn'
    namespace: 'default'
    labels: {
      format: 'k8s-extension'
    }
  }

  stringData: {
    connectionString: '${redisService.metadata.name}.${redisService.metadata.namespace}.svc.cluster.local,password=${base64ToString(redisSecret.data.redisPassword)}'
    username: redisSecret.data.redisUsername
    password: redisSecret.data.redisPassword
  }
}

// Our test framework wants an app, but we don't need an app just yet.
//
// In the future when we implements the usage of unmanaged K8s resources
// in an application we will turn this app to something useful.
resource app 'radius.dev/Application@v1alpha3' = {
  name: 'k8s-extension'
}
