import radius as radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'default'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-simulatedenv-env'
    }
    simulated: true
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-simulatedenv'
  location: location
  properties: {
    environment: env.id
  }
}

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'http-gtwy-gtwy-simulatedenv'
  location: location
  properties: {
    application: app.id
    routes: [
      {
        path: '/'
        destination: frontendRoute.id
      }
      {
        path: '/backend1'
        destination: backendRoute.id
      }
      {
        // Route /backend2 requests to the backend, and
        // transform the request to /
        path: '/backend2'
        destination: backendRoute.id
        replacePrefix: '/'
      }
    ]
  }
}

resource frontendRoute 'Applications.Core/httpRoutes@2023-10-01-preview' = {
  name: 'http-gtwy-front-rte-simulatedenv'
  location: location
  properties: {
    application: app.id
    port: 81
  }
}

resource frontendContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'http-gtwy-front-ctnr-simulatedenv'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
          provides: frontendRoute.id
        }
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: port
        path: '/healthz'
      }
    }
    connections: {
      backend: {
        source: backendRoute.id
      }
    }
  }
}

resource backendRoute 'Applications.Core/httpRoutes@2023-10-01-preview' = {
  name: 'http-gtwy-back-rte-simulatedenv'
  location: location
  properties: {
    application: app.id
  }
}

resource backendContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'http-gtwy-back-ctnr-simulatedenv'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        gatewayUrl: gateway.properties.url
      }
      ports: {
        web: {
          containerPort: port
          provides: backendRoute.id
        }
      }
      readinessProbe: {
        kind: 'httpGet'
        containerPort: port
        path: '/healthz'
      }
    }
  }
}
