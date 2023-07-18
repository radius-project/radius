import radius as radius

param application string

@description('Specifies the image to be deployed.')
param magpieimage string

resource container 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'k8s-cli-run-portforward'
  location: 'global'
  properties: {
    application: application
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 3000
        }
      }
    }
  }
}
