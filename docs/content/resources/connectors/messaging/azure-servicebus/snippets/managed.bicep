resource app 'radius.dev/Application@v1alpha3' = {
  name: 'radius-servicebus'

  //SAMPLE
  //BUS
  resource sbq 'azure.com.ServiceBusQueue' = {
    name: 'sbq'
    properties: {
      managed: true
      queue: 'orders'
    }
  }
  //BUS
  //SENDER
  resource sender 'Container' = {
    name: 'sender'
    properties: {
      container: {
        image: 'radiusteam/servicebus-sender:latest'
        env: {
          SB_CONNECTION: sbq.properties.queueConnectionString
          SB_NAMESPACE: sbq.properties.namespace
          SB_QUEUE: sbq.properties.queue
        }
      } 
      connections: {
        servicebus: {
          kind: 'azure.com/ServiceBusQueue'
          source: sbq.id
        }
      }
    }
  }
  //SENDER
  //SAMPLE

  //RECEIVER
  resource receiver 'Container' = {
    name: 'receiver'
    properties: {
      container: {
        image: 'radiusteam/servicebus-receiver:latest'
        env: {
          SB_CONNECTION: sbq.properties.queueConnectionString
          SB_NAMESPACE: sbq.properties.namespace
          SB_QUEUE: sbq.properties.queue
        }
      }
      connections: {
        servicebus: {
          kind: 'azure.com/ServiceBusQueue'
          source: sbq.id
        }
      }
    }
  }
  //RECEIVER
}
