import radius as radius

param magpieimage string
param registry string
param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-env-recipes-env-old'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-env-recipes-env-old'
    }
    recipes: {
      'Applications.Link/daprStateStores': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/dapr-state-store:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-rs-dapr-sts-recipe-old'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-rs-dapr-sts-recipe-old'
      }
    ]
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'dapr-sts-recipe-ctnr-old'
  properties: {
    application: app.id
    connections: {
      daprstatestore: {
        source: statestore.id
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
        appId: 'dapr-sts-recipe-ctnr-old'
        appPort: 3000
      }
    ]
  }
}

resource statestore 'Applications.Link/daprStateStores@2022-03-15-privatepreview' = {
  name: 'dapr-sts-recipe-old'
  properties: {
    application: app.id
    environment: env.id
  }
}
