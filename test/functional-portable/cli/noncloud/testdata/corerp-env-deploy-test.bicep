extension radius

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'test-deploy-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'default-test-deploy-env'
    }
  }
}
