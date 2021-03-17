application app = {
  name: 'azure-servicebus'

  instance db 'azure.com/ServiceBus@v1alpha1' = {
    name: 'sb'
    properties: {
      config: {
        managed: true
      }
    }
  }
}