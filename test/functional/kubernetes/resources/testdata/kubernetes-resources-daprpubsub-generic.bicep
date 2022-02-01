resource app 'radius.dev/Application@v1alpha3' = {
  name: 'kubernetes-resources-daprpubsub-generic'

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
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }
  
  resource pubsub 'dapr.io.PubSubTopic@v1alpha3' = {
    name: 'pubsub'
    properties: {
      kind: 'generic'
      type: 'pubsub.kafka'
      metadata: {
        brokers: 'dapr-kafka.kafka.svc.cluster.local:9092'
        authRequired: false
      }
      version: 'v1'
    }
  }
}


