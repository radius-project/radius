import radius as radius

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'
param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-communication-cycle'
  location: 'global'
  properties: {
    environment: environment
  }
}

resource a_route 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-communication-cycle-a-route'
  location: 'global'
  properties: {
    application: app.id
  }
}

resource a 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-communication-cycle-a'
  location: 'global'
  properties: {
    application: app.id
    connections: {
      b: {
        source: b_route.id
      }
    }
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 3000
          provides: a_route.id
        }
      }
    }
  }
}

resource b_route 'Applications.Core/httpRoutes@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-communication-cycle-b-route'
  location: 'global'
  properties: {
    application: app.id
  }
}

resource b 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'corerp-mechanics-communication-cycle-b'
  location: 'global'
  properties: {
    application: app.id
    connections: {
      a: {
        source: a_route.id
      }
    }
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: 3000
          provides: b_route.id
        }
      }
    }
  }
}
