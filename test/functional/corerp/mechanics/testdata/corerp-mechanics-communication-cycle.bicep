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

resource route_a 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'route_a'
  location: location
  properties: {
    application: app.id
  }
}

resource containerg 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'containerg'
  location: location
  properties: {
    application: app.id
    connections: {
      b: {
        source: route_b.id
      }
    }
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 3000
          provides: route_a.id
        }
      }
    }
  }
}

resource route_b 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'route_b'
  location: location
  properties: {
    application: app.id
  }
}

resource cycle_a 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'cycle_a'
  location: location
  properties: {
    application: app.id
    connections: {
      a: {
        source: route_a.id
      }
    }
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 3000
          provides: route_b.id
        }
      }
    }
  }
}
