import radius as radius

param environment string

resource app 'Applications.Core/applications@2022-03-15-privatepreview'  = {
  name: 'connectorrp-resources-dapr-pubsub-broker'
  location: 'global'
  properties:{
    environment: environment
  }
}

resource pubsubbbroker 'Applications.Connector/daprPubSubBrokers@2022-03-15-privatepreview' = {
  name: 'pubsubbbroker'
  location: 'global'

  properties: {
    environment: environment
    kind: 'generic'
    type: 'pubsub.kafka'
    metadata: {
      brokers: 'dapr-kafka.kafka.svc.cluster.local:9092'
      authRequired: false
    }
    version: 'v1'
  }
}
