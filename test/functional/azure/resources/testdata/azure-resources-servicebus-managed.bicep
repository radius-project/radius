resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-servicebus-managed'

  resource sender 'ContainerComponent' = {
    name: 'sender'
    properties: {
      connections: {
        servicebus: {
          kind: 'azure.com/ServiceBusQueue'
          source: sbq.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }

  resource sbq 'azure.com.ServiceBusQueueComponent' = {
    name: 'sbq'
    properties: {
      managed: true
      queue: 'radius-queue1'
    }
  }
}
