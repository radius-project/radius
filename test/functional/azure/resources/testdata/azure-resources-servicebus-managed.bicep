resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-servicebus-managed'

  resource sender 'Container' = {
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
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }

  resource sbq 'azure.com.ServiceBusQueue' = {
    name: 'sbq'
    properties: {
      managed: true
      queue: 'radius-queue1'
    }
  }
}
