import kubernetes from kubernetes

resource secret 'kubernetes.core/Secret@v1' = {
  metadata: {
    name: 'mysecret'
  }
  data: {
    key: 'value'
  }
}

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  resource container 'Container' = {
    name: 'mycontainer'
    properties: {
      container: {
        image: 'myimage'
        env: {
          SECRET: secret.data['key']
        }
      }
    }
  }
}
