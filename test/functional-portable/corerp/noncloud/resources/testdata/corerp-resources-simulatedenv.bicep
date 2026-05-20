extension radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

// NOTE: do NOT name this 'default'. Tests must never mutate the shared `default`
// environment because other tests deploy applications into it; the joined
// `<envNamespace>-<appName>` would exceed k8s' 63-char namespace limit and
// break unrelated tests (e.g. mechanics/communication-cycle).
resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'corerp-simulatedenv'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-simulatedenv'
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
        destination: 'http://http-gtwy-front-ctnr-simulatedenv:${port}'
      }
      {
        path: '/backend1'
        destination: 'http://http-gtwy-back-ctnr-simulatedenv:${port}'
      }
      {
        // Route /backend2 requests to the backend, and
        // transform the request to /
        path: '/backend2'
        destination: 'http://http-gtwy-back-ctnr-simulatedenv:${port}'
        replacePrefix: '/'
      }
    ]
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
        source: 'http://http-gtwy-back-ctnr-simulatedenv:${port}'
      }
    }
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
        gatewayUrl: {
          value: gateway.properties.url
        }
      }
      ports: {
        web: {
          containerPort: port
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
