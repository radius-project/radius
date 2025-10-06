extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image to be deployed.')
param magpieimage string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'group-delete-test-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'default-group-delete-test-env'
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'group-delete-test-app'
  location: location
  properties: {
    environment: env.id
  }
}

resource containerA 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'group-delete-container-a'
  location: location
  properties: {
    application: app.id
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

resource containerB 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'group-delete-container-b'
  location: location
  properties: {
    application: app.id
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
