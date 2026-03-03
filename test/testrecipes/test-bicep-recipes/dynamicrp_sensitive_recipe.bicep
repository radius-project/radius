extension kubernetes with {
  kubeConfig: ''
  namespace: context.runtime.kubernetes.namespace
} as kubernetes

param context object

// This recipe validates that sensitive fields are correctly decrypted before recipe execution.
// It reads sensitive values from context.resource.properties and stores them in a Kubernetes Secret.
// The E2E test verifies the Secret contains the expected plaintext values, proving decryption worked.

resource sensitiveSecret 'core/Secret@v1' = {
  metadata: {
    name: 'sensitive-test-${uniqueString(context.resource.id)}'
  }
  stringData: {
    password: context.resource.properties.password
    apiKey: context.resource.properties.apiKey
    secret: context.resource.properties.credentials.secret
    connectionConfigUrl: context.resource.properties.connectionConfig.url
    connectionConfigToken: context.resource.properties.connectionConfig.token
  }
}

output result object = {
  resources: [
    '/planes/kubernetes/local/namespaces/${sensitiveSecret.metadata.namespace}/providers/core/Secret/${sensitiveSecret.metadata.name}'
  ]
  values: {
    secretName: sensitiveSecret.metadata.name
  }
}
