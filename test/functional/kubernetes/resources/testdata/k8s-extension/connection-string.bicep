import kubernetes from kubernetes

resource redisService 'kubernetes.core/Service@v1' existing = {
  metadata: {
    name: 'redis-master'
    namespace: 'default'
  }
}

resource redisSecret 'kubernetes.core/Secret@v1' existing = {
  metadata: {
    name: 'redis'
    namespace: 'default'
  }
}

resource secret 'kubernetes.core/Secret@v1' = {
  metadata: {
    name: 'redis-conn'
    namespace: 'default'
    labels: {
      format: 'custom'
    }
  }

  stringData: {
    connectionString: '${redisService.metadata.name}.${redisService.metadata.namespace}.svc.cluster.local,password=${base64ToString(redisSecret.data.redisPassword)}'
  }
}

// Our test framework wants an app, but we don't need an app just yet.
//
// In the future when we implements the usage of unmanaged K8s resources
// in an application we will turn this app to something useful.
resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dummy'
}
