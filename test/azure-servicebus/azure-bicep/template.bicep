resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'radius-servicebus'

  resource sender 'Components' = {
    name: 'servicebus-sender'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'vinayada/servicebus-sender:latest'
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
    name: 'servicebus-receiver'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'vinayada/servicebus-receiver:latest'
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
        {
          name: 'servicebus-sender'
          kind: 'radius.dev/Container'
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