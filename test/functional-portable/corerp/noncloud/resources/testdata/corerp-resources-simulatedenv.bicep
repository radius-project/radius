extension radius

@description('Specifies the location for resources.')
param location string = 'local'

@description('Specifies the port for the container resource.')
param port int = 3000

@description('Specifies the image for the container resource.')
param magpieimage string

// Simulated environment: resources are recorded but never deployed to Kubernetes.
// A gateway resource is intentionally omitted as incidental scaffolding: this test only asserts
// that a simulated environment creates zero real pods.
resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'corerp-resources-simulatedenv-env'
  location: 'global'
  properties: {
    simulated: true
    providers: {
      kubernetes: {
        namespace: 'corerp-resources-simulatedenv'
      }
    }
  }
}

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'corerp-resources-simulatedenv'
  location: location
  properties: {
    environment: env.id
  }
}

resource frontendContainer 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'http-gtwy-front-ctnr-simulatedenv'
  location: location
  properties: {
    application: app.id
    environment: env.id
    containers: {
      'http-gtwy-front-ctnr-simulatedenv': {
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

resource backendContainer 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'http-gtwy-back-ctnr-simulatedenv'
  location: location
  properties: {
    application: app.id
    environment: env.id
    containers: {
      'http-gtwy-back-ctnr-simulatedenv': {
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
