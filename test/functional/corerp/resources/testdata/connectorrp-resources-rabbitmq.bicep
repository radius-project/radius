import radius as radius

param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview'  = {
  name: 'connectorrp-resources-rabbitmq'
  location: 'global'
  properties:{
    environment: environment
  }
}

resource rabbitmq 'Applications.Connector/rabbitMQMessageQueues@2022-03-15-privatepreview' = {
  name: 'rabbitMQ'
  location: 'global'

  properties: {
    environment: environment
    queue: 'testQueue'
    secrets: {
      connectionString: 'testConnectionString'
    }
  }
}
