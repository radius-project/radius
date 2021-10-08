resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-gateway'

  resource frontendhttp 'HttpRoute' = {
    name: 'exposedroute'
    properties: {
      port: 80
      gateway: {
        hostname: '*'
        path: '/foo'
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
