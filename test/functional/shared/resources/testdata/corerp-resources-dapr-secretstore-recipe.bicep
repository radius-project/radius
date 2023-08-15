import radius as radius

param magpieimage string

param location string = resourceGroup().location
param registry string
param version string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dssr-env-old'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'corerp-resources-dssr-env-old'
    }
    recipes: {
      'Applications.Link/daprSecretStores': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/functional/shared/recipes/dapr-secret-store:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-dssr-old'
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: 'corerp-resources-dssr-old'
      }
    ]
  }
}

resource myapp 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'gnrc-scs-ctnr-recipe-old'
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
        appId: 'gnrc-ss-ctnr-recipe-old'
        appPort: 3000
      }
    ]
  }
}

resource secretstore 'Applications.Link/daprSecretStores@2022-03-15-privatepreview' = {
  name: 'gnrc-scs-recipe-old'
  location: location
  properties: {
    environment: env.id
    application: app.id
  }
}
