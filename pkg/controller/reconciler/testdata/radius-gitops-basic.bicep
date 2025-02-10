extension radius

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'default'
    }
  }
}
