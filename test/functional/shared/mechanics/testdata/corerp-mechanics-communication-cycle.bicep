import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image to be deployed.')
param magpieimage string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-communication-cycle'
  location: location
  properties: {
    environment: environment
  }
}

resource mechanicsg 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'mechanicsg'
  location: location
  properties: {
    application: app.id
    connections: {
      b: {
        source: 'http://cyclea:3000'
      }
    }
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

resource cyclea 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'cyclea'
  location: location
  properties: {
    application: app.id
    connections: {
      a: {
        source: 'http://mechanicsg:3000'
      }
    }
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
