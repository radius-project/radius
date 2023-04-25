import kubernetes as kubernetes {
  kubeConfig: ''
  namespace: 'default'
}

resource ns 'core/Namespace@v1' = {
  metadata: {
    name: 'default-corerp-resources-gateway-tlstermination'
  }
}
