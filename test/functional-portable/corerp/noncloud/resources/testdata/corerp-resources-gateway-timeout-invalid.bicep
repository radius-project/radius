extension radius

@description('Specifies the environment for resources.')
param environment string

@description('Name of the Radius Application.')
param appName string

@description('Name of the Gateway resource.')
param name string

@description('Name of the Container resource.')
param ctnrName string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: appName
  properties: {
    environment: environment
  }
}

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: name
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
  name: ctnrName
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
