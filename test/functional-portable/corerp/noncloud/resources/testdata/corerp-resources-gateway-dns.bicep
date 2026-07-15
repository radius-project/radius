extension radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-gateway-dns'
  location: location
  properties: {
    environment: environment
  }
}

// This route uses L7 HTTP path-based routing to direct traffic to multiple containers. A path-rewrite rule
// (e.g. rewriting '/backend2' to the backend's '/healthz') is intentionally omitted because
// Radius.Compute/routes has no filter/rewrite support; the remaining rules preserve the core
// path-based routing intent.
resource gateway 'Radius.Compute/routes@2025-08-01-preview' = {
  name: 'http-gtwy-gtwy-dns'
  location: location
  properties: {
    application: app.id
    environment: environment
    kind: 'HTTP'
    rules: [
      {
        matches: [
          {
            httpPath: '/'
          }
        ]
        destinationContainer: {
          resourceId: frontendcontainerdns.id
          containerName: 'frontendcontainerdns'
          containerPort: port
        }
      }
      {
        matches: [
          {
            httpPath: '/backend1'
          }
        ]
        destinationContainer: {
          resourceId: backendcontainerdns.id
          containerName: 'backendcontainerdns'
          containerPort: port
        }
      }
    ]
  }
}

resource frontendcontainerdns 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'frontendcontainerdns'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      frontendcontainerdns: {
        image: magpieimage
        ports: {
          web: {
            containerPort: port
          }
        }
      }
    }
    connections: {
      backendcontainerdns: {
        source: backendcontainerdns.id
      }
    }
  }
}

resource backendcontainerdns 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'backendcontainerdns'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      backendcontainerdns: {
        image: magpieimage
        ports: {
          web: {
            containerPort: port
          }
        }
      }
    }
  }
}
