import radius as radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string = 'corerp-resources-gateway'

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param image string = 'radiusdev.azurecr.io/magpiego:latest'

var appPrefix = 'corerp-resources-gateway'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: '${appPrefix}-app'
  location: location
  properties: {
    environment: environment
  }
}

resource gateway 'Applications.Core/gateways@2022-03-15-privatepreview' = {
  name: '${appPrefix}-gateway'
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
  name: '${appPrefix}-frontend-route'
  location: location
  properties: {
    application: app.id
    port: 81
  }
}

resource frontendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: '${appPrefix}-frontend-container'
  location: location
  properties: {
    application: app.id
    container: {
      image: image
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
  name: '${appPrefix}-backend-route'
  location: location
  properties: {
    application: app.id
  }
}

resource backendContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: '${appPrefix}-backend-container'
  location: location
  properties: {
    application: app.id
    container: {
      image: image
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
  }
}
