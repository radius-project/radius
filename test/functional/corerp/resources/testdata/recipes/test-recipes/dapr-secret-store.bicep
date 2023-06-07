import kubernetes as kubernetes {
  namespace: context.runtime.kubernetes.namespace
  kubeConfig: ''
}
param context object
resource dapr 'dapr.io/Component@v1alpha1'  = {
  metadata: {
    name: 'secretstore-${uniqueString(context.resource.id)}'
    namespace: context.runtime.kubernetes.namespace
  }
  spec: {
    type: 'secretstores.kubernetes'
    version: 'v1'
    metadata:[]
  }
}

output result object = {
  values: {
    componentName: dapr.metadata.name
  }
}
