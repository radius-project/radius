resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //SAMPLE
  resource frontend 'Container' = {
    name: 'frontend'
    properties: {
      //CONTAINER
      container: {
        image: 'registry/container:tag'
      }
      //CONTAINER
      traits: [
        {
          kind: 'dapr.io/Sidecar@v1alpha1'
          appId: 'frontend'
          appPort: 3000
        }
      ]
    }
  }
  //SAMPLE

}
