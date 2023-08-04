import radius as radius

param magpieimage string
param registry string
param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'dpsb-recipe-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'dpsb-recipe-env'
    }
    recipes: {
      'Applications.Dapr/pubSubBrokers': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/dapr-pubsub-broker:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'dpsb-recipe-app'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'dpsb-recipe-app'
      }
    ]
  }
}

resource myapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'dpsb-recipe-app-ctnr'
  properties: {
    application: app.id
    connections: {
      daprpubsub: {
        source: pubsubBroker.id
      }
    }
    container: {
      image: magpieimage
      readinessProbe: {
        kind: 'httpGet'
        containerPort: 3000
        path: '/healthz'
      }
    }
    extensions: [
      {
        kind: 'daprSidecar'
        appId: 'dpsb-recipe-app-ctnr'
        appPort: 3000
      }
    ]
  }
}

resource pubsubBroker 'Applications.Dapr/pubSubBrokers@2022-03-15-privatepreview' = {
  name: 'dpsb-recipe'
  properties: {
    application: app.id
    environment: env.id
  }
}
