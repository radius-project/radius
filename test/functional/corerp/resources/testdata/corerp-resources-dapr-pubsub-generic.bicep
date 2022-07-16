import radius as radius

param magpieimage string = 'radiusdev.azurecr.io/magpiego:latest'

param environment string

param location string = resourceGroup().location

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dapr-pubsub-generic'
  location: location
  properties: {
    environment: environment
  }
}

resource publisher 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'publisher'
  location: location
  properties: {
    application: app.id
    connections: {
      daprpubsub: {
        source: pubsub.id
      }
    }
    container: {
      image: magpieimage
      readinessProbe:{
        kind:'httpGet'
        containerPort:3000
        path: '/healthz'
      }
    }
  }
}

resource pubsub 'Applications.Connector/daprPubSubBrokers@2022-03-15-privatepreview' = {
  name: 'pubsub'
  location: location
  properties: {
    environment: environment
    application: app.id
    kind: 'generic'
    type: 'pubsub.kafka'
    metadata: {
      brokers: 'dapr-kafka.kafka.svc.cluster.local:9092'
      authRequired: false
    }
    version: 'v1'
  }
}
