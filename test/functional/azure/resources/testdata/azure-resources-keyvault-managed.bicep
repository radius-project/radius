resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-keyvault-managed'

  resource kv 'azure.com.KeyVault' = {
    name: 'kv'
    properties: {
      managed: true
    }
  }

  resource kvaccessor 'Container' = {
    name: 'kvaccessor'
    properties: {
      connections: {
        keyvault: {
          kind: 'azure.com/KeyVault'
          source: kv.id
        }
      }
      container: {
        image: 'radius.azurecr.io/magpie:latest'
        readinessProbe:{
          kind:'httpGet'
          containerPort:3000
          path: '/healthz'
        }
      }
    }
  }
}
