resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-gateway-explicit'

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

  resource frontendhttp 'HttpRoute' = {
    name: 'backend'
    properties: {
      gateway: {
        source: gateway.id
        hostname: '*'
      }
    }
  }
  
  resource frontend 'ContainerComponent' = {
    name: 'exposed'
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
