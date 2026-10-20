extension radius

@description('Specifies the Kubernetes namespace where the environment deploys recipe resources.')
param envNamespace string = 'default-test-deploy-env'

resource env 'Radius.Core/environments@2025-08-01-preview' = {
  name: 'test-deploy-env'
  properties: {
    providers: {
      kubernetes: {
        namespace: envNamespace
      }
    }
  }
}
