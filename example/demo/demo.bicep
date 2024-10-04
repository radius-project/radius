// Import the set of Radius resources (Applications.*) into Bicep
extension radius

param port int
param tag string

resource demoenv 'Applications.Core/environments@2023-10-01-preview' existing = {
  name: 'demoenv'
}

resource demoapp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'demoapp'
  properties: {
    environment: demoenv.id
  }
}

resource democtnr 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'democtnr'
  properties: {
    application: demoapp.id
    container: {
      image: 'ghcr.io/radius-project/samples/demo:${tag}'
      ports: {
        web: {
          containerPort: port
        }
      }
    }
  }
}
