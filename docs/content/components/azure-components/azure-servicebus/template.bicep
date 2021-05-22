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
      dependsOn: [
        {
          name: 'sbq'
          kind: 'azure.com/ServiceBusQueue'
          setEnv: {
            SB_CONNECTION: 'connectionString'
            SB_NAMESPACE: 'namespace'
            SB_QUEUE: 'queue'
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
      dependsOn: [
        {
          name: 'sbq'
          kind: 'azure.com/ServiceBusQueue'
          setEnv: {
            SB_CONNECTION: 'connectionString'
            SB_NAMESPACE: 'namespace'
            SB_QUEUE: 'queue'
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
