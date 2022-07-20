import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param environment string = 'test'

@description('Specifies the image to be deployed.')
param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-communication-cycle'
  location: location
  properties: {
    environment: environment
  }
}

resource routea 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'routea'
  location: location
  properties: {
    application: app.id
  }
}

resource mechanicsg 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'mechanicsg'
  location: location
  properties: {
    application: app.id
    connections: {
      b: {
        source: routeb.id
      }
    }
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 3000
          provides: routea.id
        }
      }
    }
  }
}

resource routeb 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'routeb'
  location: location
  properties: {
    application: app.id
  }
}

resource cyclea 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'cyclea'
  location: location
  properties: {
    application: app.id
    connections: {
      a: {
        source: routea.id
      }
    }
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 3000
          provides: routeb.id
        }
      }
    }
  }
}
