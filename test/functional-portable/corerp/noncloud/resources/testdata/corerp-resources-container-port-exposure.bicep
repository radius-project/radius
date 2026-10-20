extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-container-port-exposure'
  location: location
  properties: {
    environment: environment
  }
}

// Each container that exposes a containerPort should get its own Kubernetes Service.
resource containerqy 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'containerqy'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      containerqy: {
        image: magpieimage
        ports: {
          web: {
            containerPort: 4000
            protocol: 'TCP' // optional: defaults to TCP
          }
        }
      }
    }
  }
}

// The optional protocol can be specified explicitly.
resource containerqu 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'containerqu'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      containerqu: {
        image: magpieimage
        ports: {
          web: {
            containerPort: 3000
            protocol: 'TCP' // optional: defaults to TCP
          }
        }
      }
    }
  }
}

// A container should still expose a port (and get a Service) when the optional protocol is omitted.
resource containerqi 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'containerqi'
  location: location
  properties: {
    application: app.id
    environment: environment
    containers: {
      containerqi: {
        image: magpieimage
        ports: {
          web: {
            containerPort: 3000
          }
        }
      }
    }
  }
}
