resource app 'radius.dev/Application@v1alpha3' = {
  name: 'radius-keyvault'

  //SAMPLE
  //KEYVAULT
  resource kv 'azure.com.KeyVault' = {
    name: 'kv'
    properties: {
      managed: true
    }
  }
  //KEYVAULT

  //ACCESSOR
  resource kvaccessor 'Container' = {
    name: 'kvaccessor'
    properties: {
      container: {
        image: 'radiusteam/azure-keyvault-app:latest'
        env: {
          KV_URI: kv.properties.uri
        }
      }
      connections: {
        vault: {
          kind: 'azure.com/KeyVault'
          source: kv.id
        }
      }
    }
  }
  //ACCESSOR
  //SAMPLE
}
