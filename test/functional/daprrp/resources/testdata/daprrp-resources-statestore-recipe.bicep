import radius as radius

param magpieimage string
param registry string
param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'daprrp-environment-recipes-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'daprrp-environment-recipes-env'
    }
    recipes: {
      'Applications.Dapr/stateStores': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/dapr-state-store:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'daprrp-resources-sts-recipe'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'daprrp-resources-sts-recipe'
      }
    ]
  }
}

resource webapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'dapr-sts-recipe-ctnr'
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
        appId: 'dapr-sts-recipe-ctnr'
        appPort: 3000
      }
    ]
  }
}

resource statestore 'Applications.Dapr/stateStores@2022-03-15-privatepreview' = {
  name: 'dapr-sts-recipe'
  properties: {
    application: app.id
    environment: env.id
  }
}
