resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-gateway-explicit'

  resource gateway 'Gateway' = {
    name: 'gateway'
    properties: {
      listeners: {
        http: {
          port: 80
          protocol: 'HTTP'
        }
      }
    }
  }

  resource backendhttp 'HttpRoute' = {
    name: 'backendhttp'
    properties: {
      gateway: {
        source: gateway.id
        hostname: '*'
        rules: {
          foo: {
            path: {
              type: 'prefix'
              value: '/'
            }
          }
        }
      }
    }
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
