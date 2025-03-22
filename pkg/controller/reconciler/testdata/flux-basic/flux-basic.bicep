extension radius

resource fluxBasicEnv 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'flux-basic-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'flux-basic'
    }
  }
}
