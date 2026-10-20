extension radius

@description('Specifies the image to be deployed.')
param magpieimage string

@description('Specifies the environment for resources.')
param environment string

// A container that references an invalid application resource ID. The deployment is
// expected to fail because the application ID cannot be parsed as a valid resource ID.
resource container 'Radius.Compute/containers@2025-08-01-preview' = {
  name: 'invalid-ctnr'
  location: 'global'
  properties: {
    application: 'not_an_id'
    environment: environment
    containers: {
      invalidctnr: {
        image: magpieimage
      }
    }
  }
}
