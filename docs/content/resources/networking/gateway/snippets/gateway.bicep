resource app 'radius.dev/Application@v1alpha3' = {
  name: 'myapp'

  //BACKEND
  resource backend 'Container' = {
    name: 'backend'
    properties: {
      container: {
        image: 'registry/container:tag'
        ports: {
          http: {
            containerPort: 80
            provides: http.id
          }
        }
      }
    }
  }
  //BACKEND

  //GATEWAY
  resource gateway 'Gateway' = {
    name: 'httproute'
    properties: {
      listeners: {
        http: {
          port: 80
          protocol: 'HTTP'
        }
      }
    }
  }
  //GATEWAY

  //ROUTE
  resource http 'HttpRoute' = {
    name: 'httproute'
    properties: {
      port: 80
      gateway: {
        source: gateway.id
        hostname: '*'
      }
    }
  }
  //ROUTE

  //FRONTEND
  resource frontend 'Container' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'registry/container:tag'
        env: {
          BACKEND_URL: http.properties.url
        }
      }
      connections: {
        http: {
          kind: 'Http'
          source: http.id
        }
      }
    }
  }
  //FRONTEND
  
}
