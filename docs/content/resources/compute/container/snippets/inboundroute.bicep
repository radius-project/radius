resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'inbound-route'

  //SAMPLE
  resource frontend 'Components' = {
    name: 'frontend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      //HIDE
      run: {
        container: {
          image: 'rynowak/frontend:0.5.0-dev'
        }
      }
      //HIDE
      bindings: {
        web: {
          kind: 'http'
          targetPort: 80
        }
      }
      traits: [
        {
          kind: 'radius.dev/InboundRoute@v1alpha1'
          binding: 'web'
        }
      ]
    }
  }
  //SAMPLE

  resource backend 'Components' = {
    name: 'backend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'rynowak/backend:0.5.0-dev'
        }
      }
      bindings: {
        web: {
          kind: 'http'
          targetPort: 80
        }
      }
    }
  }
}
