param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'azure-mechanics-communication-cycle'
}

resource a_route 'Applications.Core/httproutes@2022-03-15-privatepreview' = {
  name: 'a'
}

resource a 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'a'
  properties: {
    connections: {
      b: {
        kind: 'Http'
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

resource b_route 'Applications.Core/httproutes@2022-03-15-privatepreview' = {
  name: 'b'
}

resource b 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'b'
  properties: {
    connections: {
      a: {
        kind: 'Http'
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
