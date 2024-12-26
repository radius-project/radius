extension radius

param name string
param envId string
param namespace string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: '${name}-app'
  properties: {
    environment: envId
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: namespace
      }
    ]
  }
}

output appId string = app.id
