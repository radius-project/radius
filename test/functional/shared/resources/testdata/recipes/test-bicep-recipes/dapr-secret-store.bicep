import kubernetes as kubernetes {
  namespace: context.runtime.kubernetes.namespace
  kubeConfig: ''
}

param context object

resource dapr 'dapr.io/Component@v1alpha1' = {
  metadata: {
    name: context.resource.name
    namespace: context.runtime.kubernetes.namespace
    labels: {
      'radius.dev/application': context.application.name
      'radius.dev/resource': context.resource.name
      'radius.dev/resource-type': 'applications.dapr-secretstores'
    }
  }
  spec: {
    type: 'secretstores.kubernetes'
    version: 'v1'
    metadata: []
  }
}

output result object = {
  values: {
    componentName: dapr.metadata.name
  }
}
