extension radius

// This bicep template exercises the read-side of x-ms-client-flatten support
// in the Radius Bicep type generator. Resources are still *authored* using the
// `properties: { ... }` envelope (because Radius RP only accepts that wire
// format), but a second container resource derives its fields from the first
// by reading flattened aliases — e.g. `ctnr.container.image` instead of
// `ctnr.properties.container.image`. Those top-level aliases are emitted as
// ReadOnly projections by the type generator, so authoring at the top level
// (e.g. `ctnr.container = {...}`) is correctly rejected by Bicep.

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'corerp-resources-container-flatten'
  location: location
  properties: {
    environment: environment
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-resources-container-flatten-app'
      }
    ]
  }
}

resource ctnr 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'ctnr-ctnr-flatten'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
        }
      }
    }
    connections: {}
  }
}

// Second container that derives its fields from `ctnr` via flattened
// read-side aliases. Each `ctnr.<field>` reference here uses the top-level
// alias surfaced by the type generator (read-only projection of
// `ctnr.properties.<field>`). If the generator stopped hoisting any of these,
// Bicep compilation would fail with "property does not exist on type ..." and
// the deploy step would never run.
resource ctnr2 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'ctnr-ctnr-flatten-2'
  location: location
  properties: {
    application: ctnr.application
    container: {
      image: ctnr.container.image
      ports: {
        web: {
          containerPort: ctnr.container.ports.web.containerPort
        }
      }
    }
    connections: {}
  }
}
