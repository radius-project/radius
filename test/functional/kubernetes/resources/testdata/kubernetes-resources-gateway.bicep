param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'
param magpieport int = 3000

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
          path: '/backend1'
          destination: backendroute.id
        }
        {
          // Route /backend2 requests to the backend, and
          // transform the request to /
          path: '/backend2'
          destination: backendroute.id
          replacePrefix: '/'
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
        image: magpieimage
        ports: {
          web: {
            containerPort: magpieport
            provides: frontendroute.id
          }
        }
        readinessProbe: {
          kind: 'httpGet'
          containerPort: magpieport
          path: '/healthz'
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
        image: magpieimage
        ports: {
          web: {
            containerPort: magpieport
            provides: backendroute.id
          }
        }
        readinessProbe: {
          kind: 'httpGet'
          containerPort: magpieport
          path: '/healthz'
        }
      }
    }
  }
}
