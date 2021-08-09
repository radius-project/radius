resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'kubernetes-resources-statestore-managed'

  resource receiverapplication 'Components' = {
    name: 'sender'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radius.azurecr.io/magpie:latest'
        }
      }
      uses: [
        {
          binding: statestore.properties.bindings.default
          env: {
            BINDING_DAPRSTATESTORE_STATESTORENAME: statestore.properties.bindings.default.stateStoreName
          }
        }
      ]
      bindings: {
        web: {
          kind: 'http'
          targetPort: 3000
        }
      }
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          appId: 'receiver'
          appPort: 3000
        }
      ]
    }
  }

  resource statestore 'Components' = {
    name: 'statestore'
    kind: 'dapr.io/StateStore@v1alpha1'
    properties: {
      config: {
        kind: 'any'
        managed: true
      }
    }
  }
}
