resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-container-readiness-liveness'

  resource backend 'Container' = {
    name: 'backend'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        ports: {
          web: {
            containerPort: 80
          }
        }
        readinessProbe:{
          kind:'httpGet'
          containerPort:8080
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
}
