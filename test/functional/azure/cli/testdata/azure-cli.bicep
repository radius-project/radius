param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest' string = 'radiusdev.azurecr.io/magpiego:latest'

resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-cli'

  resource a 'Container' = {
    name: 'a'
    properties: {
      container: {
        image: magpieimage
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
          initialDelaySeconds:3
          failureThreshold:4
          periodSeconds:20
        }
        livenessProbe:{
          kind:'exec'
          command:'ls /tmp'
        }
      }
    }
  }

  resource b 'Container' = {
    name: 'b'
    properties: {
      container: {
        image: magpieimage
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }
}
