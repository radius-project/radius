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

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-gateway-sslpassthrough'
  location: location
  properties: {
    environment: environment
  }
}

// A TLS route renders a Gateway API TLSRoute attached to the managed Gateway's Passthrough :443 listener,
// matching on SNI (hostnames). The container continues to terminate its own TLS, preserving the original
// passthrough intent.
resource gateway 'Radius.Compute/routes@2025-08-01-preview' = {
  name: 'ssl-gtwy-gtwy'
  location: location
  properties: {
    application: app.id
    environment: environment
    kind: 'TLS'
    hostnames: [
      'ssl-gtwy.example.com'
    ]
    rules: [
      {
       matches: [
          {}
        ]
        destinationContainer: {
          resourceId: frontendContainer.id
          containerName: 'ssl-gtwy-front-ctnr'
          containerPort: port
        }
      }
    ]
  }
}

resource frontendContainer 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'ssl-gtwy-front-ctnr'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      'ssl-gtwy-front-ctnr': {
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
          }
        }
        readinessProbe: {
          tcpSocket: {
            port: port
          }
        }
      }
    }
  }
}
