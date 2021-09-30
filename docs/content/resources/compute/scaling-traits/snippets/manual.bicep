resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //SAMPLE
  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      //CONTAINER
      container: {
        image: 'registry/container:tag'
      }
      //CONTAINER
      traits: [
        {
          kind: 'radius.dev/ManualScaling@v1alpha1'
          replicas: 5
        }
      ]
    }
  }
  //SAMPLE

}
