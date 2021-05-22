resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'radius-keyvault'

  resource kv 'Components' = {
    name: 'kv'
    kind: 'azure.com/KeyVault@v1alpha1'
    properties: {
        config: {
            managed: true
        }
    }
  }

  resource kvaccessor 'Components' = {
    name: 'kvaccessor'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'radiusteam/azure-keyvault-app:latest'
        }
      }
      dependsOn: [
        {
          name: 'kv'
          kind: 'azure.com/KeyVault'
          setEnv: {
            KV_URI: 'kvuri'
          }
        }
      ]
    }
  }
}
