resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-container-manualscale'

  resource frontendhttp 'HttpRoute' = {
    name: 'frontend'
  }

  resource frontend 'ContainerComponent' = {
    name: 'frontend'
    properties: {
      connections: {
        backend: {
          kind: 'Http'
          source: backendhttp.id
        }
      }
      container: {
        image: 'rynowak/frontend:0.5.0-dev'
        ports: {
          web: {
            containerPort: 80
            provides: frontendhttp.id
          }
        }
        env: {
          SERVICE__BACKEND__HOST: backendhttp.properties.host
          SERVICE__BACKEND__PORT: '${backendhttp.properties.port}'
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

  resource backendhttp 'HttpRoute' = {
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
            provides: backendhttp.id
          }
        }
      }
    }
  }
}
