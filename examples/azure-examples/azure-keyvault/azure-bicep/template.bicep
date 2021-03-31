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
}
