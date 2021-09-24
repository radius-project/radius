resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-container-httpbinding'
  resource frontend_http 'HttpRoute' = {
    name: 'frontend_http'
    properties: {
      port: 80
    }
  }
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
        ports: {
          web: {
            containerPort: 80
            provides: frontend_http.id
          }
        }
        env: {
          SERVICE__BACKEND__HOST: backend_http.properties.host
          SERVICE__BACKEND__PORT: '${backend_http.properties.port}'
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
  resource backend_http 'HttpRoute' = {
    name: 'f'
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
      }
    }
  }
}
