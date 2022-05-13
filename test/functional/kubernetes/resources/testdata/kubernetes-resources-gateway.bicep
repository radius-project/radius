resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-gateway'

  resource gateway 'Gateway' = {
    name: 'gateway'
    properties: {
      routes: [
        {
          path: '/'
          destination: frontendroute.id
        }
        {
          path: '/rewriteme'
          destination: backendroute.id
          replacePrefix: '/backend'
        }
      ]
    }
  }

  resource frontendroute 'HttpRoute' = {
    name: 'frontendroute'
  }

  resource frontend 'Container' = {
    name: 'frontend'
    properties: {
      container: {
        image: 'willdavsmith/frontend:test'
        ports: {
          web: {
            containerPort: 8080
            provides: frontendroute.id
          }
        }
      }
      connections: {
        backend: {
          kind: 'Http'
          source: backendroute.id
        }
      }
    }
  }

  resource backendroute 'HttpRoute' = {
    name: 'backendroute'
  }

  resource backend 'Container' = {
    name: 'backend'
    properties: {
      container: {
        image: 'willdavsmith/backend:test'
        ports: {
          web: {
            containerPort: 8081
            provides: backendroute.id
          }
        }
      }
    }
  }
}
