extension kubernetes with {
  kubeConfig: ''
  namespace: 'default'
} as kubernetes
extension radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

@description('Specifies tls cert secret values.')
@secure()
param tlscrt string
@secure()
param tlskey string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-gateway-sslpassthrough'
  location: location
  properties: {
    environment: environment
  }
}

resource gateway 'Applications.Core/gateways@2023-10-01-preview' = {
  name: 'ssl-gtwy-gtwy'
  location: location
  properties: {
    application: app.id
    tls: { 
      sslPassthrough: true 
    } 
    routes: [
      {
        destination: 'https://${frontendContainer.name}:${frontendContainer.properties.container.ports.web.port}'
      }
    ]
  }
}

resource frontendContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'ssl-gtwy-front-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        TLS_KEY: {
          value: tlskey
        }
        TLS_CERT: {
          value: tlscrt
        }
      }
      ports: {
        web: {
          containerPort: port
          port: 443
        }
      }
      readinessProbe: {
        kind: 'tcp'
        containerPort: port
      }
    }
  }
}


