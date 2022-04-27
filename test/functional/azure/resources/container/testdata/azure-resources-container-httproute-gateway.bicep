resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-container-httproute-gateway'

  resource gateway 'Gateway' = {
    name: 'gateway'
    properties: {
      routes: [
        {
          path: '/'
          destination: frontendhttp.id
        }
        {
          path: '/backend'
          destination: backendhttp.id
        }
      ]
    }
  }

  resource frontendhttp 'HttpRoute' = {
    name: 'frontendhttp'
  }

  resource frontend 'Container' = {
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
        env: {
          SERVICE__BACKEND__HOST: backendhttp.properties.host
          SERVICE__BACKEND__PORT: string(backendhttp.properties.port)
        }
        ports: {
          web: {
            containerPort: 80
            provides: frontendhttp.id
          }
        }
      }
    }
  }

  resource backendhttp 'HttpRoute' = {
    name: 'backendhttp'
  }

  resource backend 'Container' = {
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
