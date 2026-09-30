extension radius

param environment string

resource app 'Radius.Core/applications@2025-08-01-preview' = {
  name: 'containerimages-default-app'
  properties: {
    environment: environment
  }
}

// Built by the default recipe pack's containerImages recipe, which pushes to the
// chart's in-cluster registry at localhost:31500.
resource image 'Radius.Compute/containerImages@2025-08-01-preview' = {
  name: 'defaultpackimage'
  properties: {
    environment: environment
    application: app.id
    tag: 'functest'
    build: {
      // A scratch image with no RUN steps builds for every platform without emulation.
      source: 'git::https://github.com/docker-library/hello-world.git//amd64?ref=522bcd2faf422c60b9d20e64d7cd6d56600aec97'
    }
  }
}

resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'defaultpackcntr'
  properties: {
    environment: environment
    application: app.id
    containers: {
      hello: {
        image: image.properties.imageReference
      }
    }
  }
}
