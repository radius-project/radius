application app = {
  name: 'dapr-hello'

  instance nodeapp 'radius.dev/Container@v1alpha1' = {
    name: 'nodeapp'
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
  
  instance pythonapp 'radius.dev/Container@v1alpha1' = {
    name: 'pythonapp'
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
  deployment default = {
    name: 'default'
    properties: {
      components: [
        {
          componentName: statestore.name
        }
      ]
    }
  }
  
  component statestore 'dapr.io/Component@v1alpha1' = {
    name: 'statestore'
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
      // provides: [
      //   {
      //     name: 'statestore'
      //     kind: 'dapr.io/StateStore'
      //   }
      // ]
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