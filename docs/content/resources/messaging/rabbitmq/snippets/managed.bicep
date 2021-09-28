resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'kubernetes-resources-rabbitmq-managed'
  
  resource webapp 'Components' = {
    name: 'todoapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      //HIDE
      run: {
        container: {
          image: 'radius.azurecr.io/magpie:latest'
        }
      }
      //HIDE
      uses: [
        {
          binding: rabbitmq.properties.bindings.rabbitmq
          env: {
            BINDING_RABBITMQ_CONNECTIONSTRING: rabbitmq.properties.bindings.rabbitmq.connectionString
          }
        }
      ]
    }
  }

  //SAMPLE
  resource rabbitmq 'Components' = {
    name: 'rabbitmq'
    kind: 'rabbitmq.com/MessageQueue@v1alpha1'
    properties: {
      config: {
        managed: true
        queue: 'radius-queue'
      }
    }
  }
  //SAMPLE
}
