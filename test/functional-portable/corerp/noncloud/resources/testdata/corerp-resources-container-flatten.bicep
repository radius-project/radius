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

resource ctnr 'Applications.Core/containers@2023-10-01-preview' = {
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

// Second container that references flattened fields on `ctnr` via cross-resource
// references. Each `ctnr.<field>` access exercises the read-side of flatten:
//   - ctnr.application                          (was ctnr.properties.application)
//   - ctnr.container.image                      (was ctnr.properties.container.image)
//   - ctnr.container.ports.web.containerPort    (was ctnr.properties.container.ports.web.containerPort)
// If the type generator had failed to hoist any of these onto the resource
// body, Bicep would refuse to compile this template ("property does not exist
// on type ...") and the deploy step would error out before reaching the
// cluster, so this resource doubles as a smoke test for the read side of
// flattening.
resource ctnr2 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'ctnr-ctnr-flatten-2'
  location: location
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
