extension radius

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: 'default'
  properties: {
    compute: {
      kind: 'kubernetes'
      namespace: 'default'
    }
  }
}
