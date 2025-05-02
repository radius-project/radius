extension radius

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-gateway-timeout'
  properties: {
    environment: environment
  }
}

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'timeout-gtwy-gtwy'
  properties: {
    application: app.id
    routes: [
      {
        path: '/'
        destination: 'http://timeout-gtwy-front-ctnr:81'
        timeoutPolicy: {
          request: '30potatoes'
        }
      }
    ]
  }
}

resource frontendContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'timeout-gtwy-front-ctnr'
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
  }
}
