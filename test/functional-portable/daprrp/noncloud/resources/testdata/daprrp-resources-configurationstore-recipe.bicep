extension radius

param magpieimage string
param registry string
param version string
param namespace string = 'default'
param baseName string = 'dcs-recipe'

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: '${baseName}-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: namespace
    }
    recipes: {
      'Applications.Dapr/configurationStores': {
        default: {
          templateKind: 'bicep'
          templatePath: '${registry}/test/testrecipes/test-bicep-recipes/dapr-configuration-store:${version}'
        }
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: baseName
  properties: {
    environment: env.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: baseName
      }
    ]
  }
}

resource myapp 'Applications.Core/containers@2023-10-01-preview' = {
  name: '${baseName}-ctnr'
  properties: {
    application: app.id
    connections: {
      daprconfigurationstore: {
        source: configStore.id
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
        appId: '${baseName}-ctnr'
        appPort: 3000
      }
    ]
  }
}

resource configStore 'Applications.Dapr/configurationStores@2023-10-01-preview' = {
  name: '${baseName}-cpn'
  properties: {
    application: app.id
    environment: env.id
  }
}
