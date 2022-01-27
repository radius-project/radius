resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-rabbitmq-managed'
  
  resource webapp 'Container' = {
    name: 'todoapp'
    properties: {
      //HIDE
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        env: {
          BINDING_RABBITMQ_CONNECTIONSTRING: rabbitmq.connectionString()
        }
      }
      //HIDE
      connections: {
        messages: {
          kind: 'rabbitmq.com/MessageQueue'
          source: rabbitmq.id
        }
      }
    }
  }

  //SAMPLE
  resource rabbitmq 'rabbitmq.com.MessageQueue' = {
    name: 'rabbitmq'
    properties: {
      managed: true
      queue: 'radius-queue'
    }
  }
  //SAMPLE
}
