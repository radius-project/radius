resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'radius-servicebus'

  resource sender 'Components' = {
    name: 'sender'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/servicebus-sender:latest'
        }
      }
      uses: [
        {
          binding: sbq.properties.bindings.default
          env: {
            SB_CONNECTION: sbq.properties.bindings.default.connectionString
            SB_NAMESPACE: sbq.properties.bindings.default.namespace
            SB_QUEUE: sbq.properties.bindings.default.queue
          }
        }
      ]
    }
  }

  resource receiver 'Components' = {
    name: 'receiver'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/servicebus-receiver:latest'
        }
      }
      uses: [
        {
          binding: sbq.properties.bindings.default
          env: {
            SB_CONNECTION: sbq.properties.bindings.default.connectionString
            SB_NAMESPACE: sbq.properties.bindings.default.namespace
            SB_QUEUE: sbq.properties.bindings.default.queue
          }
        }
      ]
    }
  }

  resource sbq 'Components' = {
    name: 'sbq'
    kind: 'azure.com/ServiceBusQueue@v1alpha1'
    properties: {
      config: {
        managed: true
        queue: 'radius-queue1'
      }
    }
  }
}
