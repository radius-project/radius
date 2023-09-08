import radius as radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-gateway-kme'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
        kind: 'kubernetesMetadata'
        annotations: {
          'user.ann.1': 'user.ann.val.1'
          'user.ann.2': 'user.ann.val.2'
        }
        labels: {
          'user.lbl.1': 'user.lbl.val.1'
          'user.lbl.2': 'user.lbl.val.2'
        }
      }
    ]
  }
}

resource gateway 'Applications.Core/gateways@2022-03-15-privatepreview' = {
  name: 'http-gtwy-kme'
  location: location
  properties: {
    application: app.id
    routes: [
      {
        path: '/'
        destination: 'http://http-gtwy-front-ctnr-kme:81'
      }
      {
        path: '/backend1'
        destination: 'http://http-gtwy-back-ctnr-kme:3000'
      }
      {
        // Route /backend2 requests to the backend, and
        // transform the request to /
        path: '/backend2'
        destination: 'http://http-gtwy-back-ctnr-kme:3000'
        replacePrefix: '/'
      }
    ]
  }
}

resource frontendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'http-gtwy-front-ctnr-kme'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
          port: 81
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
        source: 'http://http-gtwy-back-ctnr-kme:3000'
      }
    }
  }
}

resource backendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'http-gtwy-back-ctnr-kme'
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
