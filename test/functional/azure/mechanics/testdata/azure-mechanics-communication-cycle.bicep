resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-mechanics-communication-cycle'

  resource a_route 'HttpRoute' = {
    name: 'a'
  }

  resource a 'Container' = {
    name: 'a'
    properties: {
      connections: {
        b: {
          kind: 'Http'
          source: b_route.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        ports: {
          web: {
            containerPort: 3000
            provides: a_route.id
          }
        }
      }
    }
  }

  resource b_route 'HttpRoute' = {
    name: 'b'
  }

  resource b 'Container' = {
    name: 'b'
    properties: {
      connections: {
        a: {
          kind: 'Http'
          source: a_route.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        ports: {
          web: {
            containerPort: 3000
            provides: b_route.id
          }
        }
      }
    }
  }
}
