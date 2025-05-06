extension radius

param replicas string

resource fluxUpdateEnv 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'flux-update-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'flux-update'
    }
  }
}

resource fluxUpdateApp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'flux-update-app'
  properties: {
    environment: fluxUpdateEnv.id
  }
}

resource fluxUpdateContainer 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'flux-update-container'
  properties: {
    application: fluxUpdateApp.id
    container: {
      image: 'nginx'
    }
    extensions: [
      {
        kind: 'manualScaling'
        replicas: int(replicas)
      }
    ]
  }
}
