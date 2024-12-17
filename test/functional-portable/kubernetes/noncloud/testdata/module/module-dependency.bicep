extension radius

param name string
param namespace string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: name
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: namespace
    }
  }
}

output envId string = env.id
