resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-hello'

  resource nodeapp 'Components' = {
    name: 'nodeapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'rynowak/dapr-hello-nodeapp:latest'
        }
      }
      provides: [
        {
          name: 'nodeapp'
          kind: 'http'
          containerPort: 3000
        }
      ]
      // dependsOn: [
      //   {
      //     name: 'statestore'
      //     kind: 'dapr.io/StateStore'
      //   }
      // ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'nodeapp'
            appPort: 3000
          }
        }
      ]
    }
  }
  
  resource pythonapp 'Components' = {
    name: 'pythonapp'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          image: 'rynowak/dapr-hello-pythonapp:latest'
        }
      }
      // dependsOn: [
      //   {
      //     name: 'nodeapp'
      //     kind: 'dapr.io/Invoke'
      //   }
      // ]
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'pythonapp'
          }
        }
      ]
    }
  }
  
  // Imagine this deployment in a separate file/repo
  resource default 'Deployments' = {
    name: 'default'
    properties: {
      components: [
        {
          componentName: statestore.name
        }
      ]
    }
  }
  
  resource statestore 'Components' = {
    name: 'statestore'
    kind: 'dapr.io/Component@v1alpha1'
    properties: {
      config: {
        type: 'state.azure.tablestorage'
        metadata: [
          {
            name: 'accountName'
            value: stg.name
          }
          {
            name: 'accountKey'
            value: listKeys(stg.id, stg.apiVersion).keys[0].value
          }
          {
            name: 'tableName'
            value: 'dapr'
          }
        ]
      }
    }
  }
}

resource stg 'microsoft.storage/storageAccounts@2019-06-01' = {
  name: 'storage${uniqueString(resourceGroup().id)}'
  location: resourceGroup().location
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
}

resource table 'microsoft.storage/storageAccounts/tableServices/tables@2019-06-01' = {
  name: '${stg.name}/default/dapr'
}
