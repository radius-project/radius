resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-cli'

  resource a 'Container' = {
    name: 'a'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }

  resource b 'Container' = {
    name: 'b'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }

  resource e 'Container' = {
    name: 'e'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }

  resource d 'HttpRoute' = {
    name: 'd'
    properties: {
      gateway: {
        hostname: '*'
      }
    }
  }

  resource c 'HttpRoute' = {
    name: 'c'
    properties: {
      gateway: {
        hostname: '*'
      }
    }
  }
}
