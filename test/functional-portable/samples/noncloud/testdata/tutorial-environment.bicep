extension radius

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'tutorial'
  properties: {
    providers: {
      kubernetes: {
        namespace: 'tutorial'
      }
    }
  }
}
