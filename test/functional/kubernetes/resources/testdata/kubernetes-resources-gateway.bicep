resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-gateway'

  resource backendgateway 'Gateway' = {
    name: 'backendgateway'
    properties: {
      routes: [
        {
          path: '/frontend'
          destination: frontendhttp.id
        }
        {
          path: '/backend'
          destination: backendhttp.id
        }
      ]
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

  resource frontendhttp 'HttpRoute' = {
    name: 'frontendhttp'
  }
  
  resource frontend 'Container' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'rynowak/frontend:0.5.0-dev'
        ports: {
          web: {
            containerPort: 80
            provides: frontendhttp.id
          }
        }
      }
    }
  }
}
