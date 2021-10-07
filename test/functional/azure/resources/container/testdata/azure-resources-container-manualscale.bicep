resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-container-manualscale'

  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      connections: {
        backend: {
          kind: 'Http'
          source: backend_http.id
        }
      }
      container: {
        image: 'rynowak/frontend:0.5.0-dev'
        env: {
          SERVICE__BACKEND__HOST: backend_http.properties.host
          SERVICE__BACKEND__PORT: string(backend_http.properties.port)
        }
      }
    }
  }

  resource backend_http 'HttpRoute' = {
    name: 'backend'
  }

  resource backend 'ContainerComponent' = {
    name: 'backend'
    properties: {
      container: {
        image: 'rynowak/backend:0.5.0-dev'
        ports: {
          web: {
            containerPort: 80
            provides: backend_http.id
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
      traits: [
        {
          kind: 'radius.dev/ManualScaling@v1alpha1'
          replicas: 2
        }
      ]
    }
  }
}
