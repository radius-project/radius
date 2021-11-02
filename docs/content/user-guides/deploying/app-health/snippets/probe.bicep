resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

//SAMPLE
  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'registry/container:tag'
        readinessProbe:{
          kind:'httpGet'
          containerPort:8080
          path: '/healthz'
          initialDelaySeconds:3
          failureThreshold:4
          periodSeconds:20
        }
      }
    }
  }
  //SAMPLE

}

