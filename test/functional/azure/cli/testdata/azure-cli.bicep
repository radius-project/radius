resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-cli'

  resource a 'Container' = {
    name: 'a'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }

  resource b 'Container' = {
    name: 'b'
    properties: {
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }
}
