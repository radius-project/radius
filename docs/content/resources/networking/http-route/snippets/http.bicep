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

  //ROUTE
  resource http 'HttpRoute' = {
    name: 'httproute'
    properties: {
      port: 80
      gateway: {
        hostname: '*'
        rules: {
          http: {
            path: {
              value: '/foo'
              type: 'prefix'
            }
          }
        }
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
