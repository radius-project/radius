extension radius

param appId string
param containerImage string
param replicas string

resource fluxComplexCtnr 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'flux-complex-container'
  properties: {
    application: appId
    container: {
      image: containerImage
    }
    extensions: [
      {
        kind: 'manualScaling'
        replicas: int(replicas)
      }
    ]
  }
}
