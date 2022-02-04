resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-dapr-pubsub-generic'

  resource publisher 'Container' = {
    name: 'publisher'
    properties: {
      connections: {
        daprpubsub: {
          kind: 'dapr.io/PubSubTopic'
          source: pubsub.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
      }
    }
  }

  resource kafkaRoute 'HttpRoute' existing = {
    name: 'kafka-route'
  }

  //SAMPLE
  resource pubsub 'dapr.io.PubSubTopic@v1alpha3' = {
    name: 'pubsub'
    properties: {
      kind: 'generic'
      type: 'pubsub.kafka'
      metadata: {
        brokers: kafkaRoute.properties.url
        authRequired: false
        consumeRetryInternal: 1024
      }
      version: 'v1'
    }
  }
  //SAMPLE
}
