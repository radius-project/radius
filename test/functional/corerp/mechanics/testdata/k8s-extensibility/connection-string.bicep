import kubernetes as kubernetes {
  namespace: 'corerp-mechanics-k8s-extensibility'
  kubeConfig: ''
}

resource redisService 'core/Service@v1' existing = {
  metadata: {
    name: 'redis-master'
    namespace: 'corerp-mechanics-k8s-extensibility'
  }
}

resource redisSecret 'core/Secret@v1' existing = {
  metadata: {
    name: 'redis'
    namespace: 'corerp-mechanics-k8s-extensibility'
  }
}

resource secret 'core/Secret@v1' = {
  metadata: {
    name: 'redis-conn'
    namespace: 'corerp-mechanics-k8s-extensibility'
    labels: {
      format: 'k8s-extension'
    }
  }

  stringData: {
    connectionString: '${redisService.metadata.name}.${redisService.metadata.namespace}.svc.cluster.local,password=${base64ToString(redisSecret.data.redisPassword)}'
  }
}
