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
  name: 'corerp-resources-gateway'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'default-corerp-resources-gateway'
      }
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
  name: 'http-gtwy-gtwy'
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

resource frontendRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'http-gtwy-front-rte'
  location: location
  properties: {
    application: app.id
    port: 81
  }
}

resource frontendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'http-gtwy-front-ctnr'
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

resource backendRoute 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'http-gtwy-back-rte'
  location: location
  properties: {
    application: app.id
  }
}

resource backendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'http-gtwy-back-ctnr'
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
