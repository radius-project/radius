application app = {
  name: 'azure-servicebus'

  instance webapp 'radius.dev/Container@v1alpha1' = {
    name: 'todoapp'
    properties: {
      run: {
        container: {
          image: 'rynowak/node-todo:latest'
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

  instance sb 'azure.com/ServiceBus@v1alpha1' = {
    name: 'sb'
    properties: {
      config: {
        managed: true
        namespace: 'radius-namespace1'
        queue: 'radius-queue1'
      }
    }
  }
}