extension radius

param replicas string
param containerImage string

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
  name: 'flux-complex-app-2'
  properties: {
    environment: fluxComplexEnv.id
  }
}

module fluxComplexDependency 'flux-complex-dependency.bicep' = {
  name: 'flux-complex-dependency-2'
  params: {
    appId: fluxComplexApp.id
    replicas: replicas
    containerImage: containerImage
  }
}
