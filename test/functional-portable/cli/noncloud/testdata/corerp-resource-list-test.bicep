extension radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image to be deployed.')
param magpieimage string

resource testEnv 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'test-environment'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      namespace: 'test-environment'
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'resource-list-test'
  location: location
  properties: {
    environment: testEnv.id
  }
}

resource containerA 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'containerA'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}

resource containerB 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'containerB'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
    }
  }
}

resource secretStore 'Applications.Core/secretStores@2023-10-01-preview' = {
  name: 'test-secretstore'
  location: location
  properties: {
    application: app.id
    type: 'generic'
    data: {
      'test-secret': {
        value: 'test-secret-value'
      }
    }
  }
}
