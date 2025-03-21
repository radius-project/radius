extension radius

resource fluxComplexEnv 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'flux-complex-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'flux-complex'
    }
  }
}

resource fluxComplexApp 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'flux-complex-app'
  properties: {
    environment: fluxComplexEnv.id
  }
}
