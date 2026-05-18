extension radius

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'default-containers-env'
  location: 'global'
  properties: {
    providers: {
      kubernetes: {
        namespace: 'default-containers-ns'
      }
    }
  }
}

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'default-containers-app'
  properties: {
    environment: env.id
  }
}

// Deploy a minimal Radius.Compute/containers resource using the default recipe.
// This validates the end-to-end path: manifest loaded at startup -> type
// registered -> recipe available -> container deployed.
resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'default-container'
  properties: {
    environment: env.id
    application: app.id
    containers: {
      web: {
        image: 'ghcr.io/radius-project/mirror/debian:latest'
        command: ['/bin/sh']
        args: ['-c', 'while true; do echo hello; sleep 10;done']
        ports: {
          http: {
            containerPort: 8080
          }
        }
      }
    }
  }
}

// Deploy a Radius.Compute/routes resource that routes to the container above.
// Including a second type from the same Radius.Compute namespace validates that
// both types are registered correctly when loaded from separate manifest files.
resource route 'Radius.Compute/routes@2025-08-01-preview' = {
  name: 'default-route'
  properties: {
    environment: env.id
    application: app.id
    rules: [
      {
        matches: [
          {
            httpPath: '/'
          }
        ]
        destinationContainer: {
          resourceId: container.id
          containerName: 'web'
          containerPort: 8080
        }
      }
    ]
  }
}
