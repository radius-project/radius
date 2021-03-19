application app = {
  name: 'radius-servicebus'

  instance sender 'radius.dev/Container@v1alpha1' = {
    name: 'servicebus-sender'
    properties: {
      run: {
        container: {
          image: 'vinayada/servicebus-sender:latest'
        }
      }
      dependsOn: [
        {
          name: 'sb'
          kind: 'azure.com/ServiceBus'
          setEnv: {
            SB_CONNECTION: 'connectionString'
          }
        }
      ]
    }
  }

  instance receiver 'radius.dev/Container@v1alpha1' = {
    name: 'servicebus-receiver'
    properties: {
      run: {
        container: {
          image: 'vinayada/servicebus-receiver:latest'
        }
      }
      dependsOn: [
        {
          name: 'sb'
          kind: 'azure.com/ServiceBus'
          setEnv: {
            SB_CONNECTION: 'connectionString'
          }
        }
        {
          name: 'servicebus-sender'
          kind: 'radius.dev/Container'
        }
      ]
    }
  }

  instance sb 'azure.com/ServiceBus@v1alpha1' = {
    name: 'sb'
    properties: {
        config: {
            managed: true
            queue: 'radius-queue1'
        }
    }
  }
}