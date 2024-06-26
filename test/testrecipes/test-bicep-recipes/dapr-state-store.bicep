provider kubernetes with {
  namespace: context.runtime.kubernetes.namespace
  kubeConfig: ''
} as kubernetes

param context object

module redis '../../../test/testrecipes/modules/redis-selfhost.bicep' = {
  name: 'redis-${uniqueString(context.resource.id)}'
  params: {
    name: 'redis-${uniqueString(context.resource.id)}'
    namespace: context.runtime.kubernetes.namespace
    application: context.application.name
  }
}

resource dapr 'dapr.io/Component@v1alpha1' = {
  metadata: {
    name: context.resource.name
    namespace: context.runtime.kubernetes.namespace
  }
  spec: {
    type: 'state.redis'
    metadata: [
      {
        name: 'redisHost'
        value: '${redis.outputs.host}:${redis.outputs.port}'
      }
      {
        name: 'redisPassword'
        value: ''
      }
    ]
    version: 'v1'
  }
}

output result object = {
  resources: redis.outputs.resources
  values: {
    componentName: dapr.metadata.name
  }
}
