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
      dependsOn: [
        {
          name: 'statestore'
          kind: 'dapr.io/StateStore'
        }
      ]
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
      dependsOn: [
        {
          name: 'nodeapp'
          kind: 'dapr.io/Invoke'
        }
      ]
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
          componentName: pubsub.name
        }
        {
          componentName: statestore.name
        }
      ]
    }
  }

  component pubsub 'dapr.io/Component@v1alpha1' = {
    application: app.name
    name: 'pubsub'
    properties: {
      config: {
        type: 'pubsub.azure.servicebus'
        managed: true
      }
    }
  }
  
  component statestore 'dapr.io/Component@v1alpha1' = {
    application: app.name
    name: 'statestore'
    properties: {
      config: {
        type: 'state.azure.tablestorage'
        id: table.id
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