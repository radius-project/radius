param AZURESERVICEBUSENABLED string = 'False'

//REST
//REST

resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  //REST
  //REST

  resource rabbitmq 'rabbitmq.com.MessageQueueComponent' = {
    name: 'rabbitmq'
    properties: {
      managed: true
      queue: 'eshop_event_bus'
    }
  }

}
//ESHOP
resource app 'radius.dev/Application@v1alpha3' = {
  name: AZURESERVICEBUSENABLED
}
//ESHOP
