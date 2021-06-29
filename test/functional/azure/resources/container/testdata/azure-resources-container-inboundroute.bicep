resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'azure-resources-container-inboundroute'

  resource frontend 'Components' = {
    name: 'frontend'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'rynowak/frontend:0.5.0-dev'
        }
      }
      uses: [
        {
          binding: backend.properties.bindings.web
          env: {
            SERVICE__BACKEND__HOST: backend.properties.bindings.web.host
            SERVICE__BACKEND__PORT: backend.properties.bindings.web.port
          }
        }
      ]
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
