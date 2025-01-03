extension radius

param name string
param namespace string

resource env 'Applications.Core/environments@2023-10-01-preview' = {
  name: '${name}-env'
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: '${name}-env'
    }
  }
}

module module 'module-dependency.bicep' = {
  name: 'module'
  params: {
    name: name
    envId: env.id
    namespace: namespace
  }
}
