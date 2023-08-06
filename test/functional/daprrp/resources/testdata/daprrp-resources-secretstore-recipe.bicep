import radius as radius

param magpieimage string

param location string = resourceGroup().location
param registry string
param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'daprrp-env-secretstore-recipes-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'daprrp-env-secretstore-recipes-env'
    }
    recipes: {
      'Applications.Dapr/secretStores': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/dapr-secret-store:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'daprrp-rs-secretstore-recipe'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'daprrp-rs-secretstore-recipe'
      }
    ]
  }
}

resource myapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'gnrc-scs-ctnr-recipe'
  location: location
  properties: {
    application: app.id
    connections: {
      daprsecretstore: {
        source: secretstore.id
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
        appId: 'gnrc-ss-ctnr-recipe'
        appPort: 3000
      }
    ]
  }
}

resource secretstore 'Applications.Dapr/secretStores@2022-03-15-privatepreview' = {
  name: 'gnrc-scs-recipe'
  location: location
  properties: {
    environment: env.id
    application: app.id
  }
}
