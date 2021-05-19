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
      uses: [
        {
          binding: kv.properties.bindings.default
          env: {
            KV_URI: kv.properties.bindings.default.uri
          }
        }
      ]
    }
  }
}
