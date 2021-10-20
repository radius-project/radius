import kubernetes from kubernetes

resource redisService 'kubernetes.core/Service@v1' existing = {
  metadata: {
    name: 'redis-master'
  }
}

resource redisSecret 'kubernetes.core/Secret@v1' existing = {
  metadata: {
    name: 'redis'
  }
}

resource connectionString 'kubernetes.core/Secret@v1' = {
  metadata: {
    name: 'redis-conn'
    namespace: 'default'
    labels: {
      format: 'custom'
    }
  }

  data: {
    'connectionString': '${redisService.metadata.name}.${redisService.metadata.namespace}.svc.cluster.local,password=${redisSecret.data.redisPassword}'
  }
}

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'dummy'
}
