extension radius

// This bicep template exercises the flattened authoring syntax enabled by
// honoring x-ms-client-flatten in the Radius Bicep type generator. Fields that
// previously had to live under a .properties{} envelope (environment,
// extensions, application, container, connections, ...) are written directly
// at the resource level here.

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
  environment: environment
  extensions: [
    {
      kind: 'kubernetesNamespace'
      namespace: 'corerp-resources-container-flatten-app'
    }
  ]
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'ctnr-ctnr-flatten'
  location: location
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

// Read-side flatten checks: each of these reads a field that previously lived
// under a `.properties.` envelope. If the type generator failed to hoist any
// of these onto the resource body, Bicep compilation would fail with
// "property does not exist on type ..." and the deployment step would error
// out before reaching the cluster. They double as a smoke test that flat
// access works symmetrically for authoring and for cross-resource references.
output appEnvironment string = app.environment
output containerAppId string = container.application
output containerImage string = container.container.image
output containerPort int = container.container.ports.web.containerPort
