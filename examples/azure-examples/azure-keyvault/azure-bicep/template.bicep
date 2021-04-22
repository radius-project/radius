resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'radius-keyvault'

  resource kv 'Components' = {
    name: 'kv'
    kind: 'azure.com/KeyVault@v1alpha1'
    properties: {
        config: {
            managed: true
            keypermissions: [
              'list'
              'get'
              'create'
              'delete'
            ]
            secretpermissions: [
              'list'
              'get'
              'set'
              'delete'
            ]
            certificatepermissions: [
              'list'
              'get'
              'create'
              'delete'
            ]
        }
    }
  }

  resource kvaccessor 'Components' = {
    name: 'kvaccessor'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'vinayada/azure-keyvault-app:latest'
        }
      }
      dependsOn: [
        {
          name: 'kv'
          kind: 'azure.com/KeyVault'
          setEnv: {
            KV_URI: 'uri'
          }
          set: {
            MSI_ID: 'msiId'
            MSI_APPID: 'msiAppId'
            MSI_OBJECTID: 'msiObjectId'
          }
        }
      ]
    }
  }
}
