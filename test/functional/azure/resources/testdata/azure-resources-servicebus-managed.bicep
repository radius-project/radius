resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'azure-resources-servicebus-managed'

  resource sender 'Components' = {
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
          binding: sbq.properties.bindings.default
          env: {
            BINDING_SERVICEBUS_CONNECTIONSTRING: sbq.properties.bindings.default.connectionString
            BINDING_SERVICEBUS_QUEUE: sbq.properties.bindings.default.queue
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
