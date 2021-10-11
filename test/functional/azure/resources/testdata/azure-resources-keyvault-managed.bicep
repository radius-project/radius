resource app 'radius.dev/Application@v1alpha3' = {
  name: 'azure-resources-keyvault-managed'

  resource kv 'azure.com.KeyVaultComponent' = {
    name: 'kv'
    properties: {
      managed: true
    }
  }

  resource kvaccessor 'ContainerComponent' = {
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
      }
    }
  }
}
