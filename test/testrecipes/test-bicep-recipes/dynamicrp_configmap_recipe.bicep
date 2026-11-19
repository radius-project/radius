extension kubernetes with {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
} as kubernetes

param context object

resource configMap 'core/ConfigMap@v1' = {
  metadata: {
    name: context.resource.properties.physicalName
  }
  data: {
    resource: context.resource.id
  }
}

output result object = {
  // The deployment engine omits Kubernetes resources from its output, so the recipe returns the ID.
  resources: [
    '/planes/kubernetes/local/namespaces/${configMap.metadata.namespace}/providers/core/ConfigMap/${configMap.metadata.name}'
  ]
}
