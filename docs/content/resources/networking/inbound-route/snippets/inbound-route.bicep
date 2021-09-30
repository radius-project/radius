resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //SAMPLE
  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      container: {
        image: 'registry/container:tag'
        ports: {
          web: {
            containerPort: 80
          }
        }
      }
      traits: [
        {
          kind: 'radius.dev/InboundRoute@v1alpha1'
          binding: 'web'
        }
      ]
    }
  }
  //SAMPLE

}
