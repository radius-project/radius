resource app 'radius.dev/Applications@v1alpha1' = {
  name: 'dapr-integrations'

  resource viewer 'Components' = {
    name: 'viewer'
    kind: 'azure.com/WebApp@v1alpha1'
    properties: {
      run: {
        code: {
          containers: [
            {
              name: 'viewer'
              image: 'rynowak/viewer:latest'
              ports: [
                {
                  name: 'http'
                  containerPort: 80
                }
              ]
            }
          ]
        }
        scaleOptions: {
          maxReplicas: 1
          minReplicas: 1
        }
      }
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'viewer'
            appPort: 80
            config: 'tracing'
          }
        }
      ]
    }
  }

  resource processor 'Components' = {
    name: 'processor'
    kind: 'radius.dev/Container@v1alpha1'
    properties: {
      run: {
        container: {
          name: 'processor'
          image: 'rynowak/processor:latest'
          env: [
            {
              name: 'STORAGE_ACCOUNT_NAME'
              value: stg.name
            }
            {
              name: 'STORAGE_ACCOUNT_KEY'
              value: listKeys(stg.id, stg.apiVersion).keys[0].value
            }
          ]
          ports: [
            {
              name: 'http'
              containerPort: 50003
              protocol: 'TCP'
            }
          ]
        }
      }
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'processor'
            appPort: 50003
            config: 'tracing'
            protocol: 'grpc'
          }
        }
      ]
    }
  }
  
  resource auditor 'Components' = {
    name: 'auditor'
    kind: 'azure.com/Function@v1alpha1'
    properties: {
      run: {
        code: {
          containers: [
            {
              name: 'auditor'
              image: 'rynowak/auditor:latest'
              ports: [
                {
                  name: 'http'
                  containerPort: 80
                }
              ]
              env: [
                {
                  name: 'StateStore'
                  value: 'auditstore'
                }
                {
                  name: 'StateKey'
                  value: 'id'
                }
                {
                  name: 'PubSubName'
                  value: 'pubsub'
                }
                {
                  name: 'TopicName'
                  value: 'cancellations'
                }
              ]
            }
          ]
        }
      }
      traits: [
        {
          kind: 'dapr.io/App@v1alpha1'
          properties: {
            appId: 'auditor'
            appPort: 3001
            config: 'tracing'
          }
        }
      ]
    }
  }
  
  resource auditstore 'Components' = {
    name: 'auditstore'
    kind: 'dapr.io/Component@v1alpha1'
    properties: {
      config: {
        type: 'state.azure.cosmosdb'
        metadata: [
          {
            name: 'url'
            value: auditstorecosmosaccount.properties.documentEndpoint 
          }
          {
            name: 'masterKey'
            value: listKeys(auditstorecosmosaccount.id, auditstorecosmosaccount.apiVersion).primaryMasterKey
          }
          {
            name: 'database'
            value: 'audits'
          }
          {
            name: 'collection'
            value: 'record'
          }
        ]
      }
    }
  }
  
  resource orderstore 'Components' = {
    name: 'orderstore'
    kind: 'dapr.io/Component@v1alpha1'
    properties: {
      config: {
        type: 'state.azure.cosmosdb'
        metadata: [
          {
            name: 'url'
            value: orderstorecosmosaccount.properties.documentEndpoint 
          }
          {
            name: 'masterKey'
            value: listKeys(orderstorecosmosaccount.id, orderstorecosmosaccount.apiVersion).primaryMasterKey
          }
          {
            name: 'database'
            value: 'orders'
          }
          {
            name: 'collection'
            value: 'record'
          }
        ]
      }
    }
  }
  
  resource pubsub 'Components' = {
    name: 'pubsub'
    kind: 'dapr.io/Component@v1alpha1'
    properties: {
      config: {
        type: 'pubsub.azure.servicebus'
        metadata: [
          {
            name: 'connectionString'
            value: listkeys(resourceId('microsoft.servicebus/namespaces/authorizationRules', sb.name, 'RootManageSharedAccessKey'), '2017-04-01').primaryConnectionString
          }
        ]
      }
    }
  }
  
  resource email 'Components' = {
    name: 'email'
    kind: 'dapr.io/Component@v1alpha1'
    properties: {
      config: {
        type: 'bindings.twilio.sendgrid'
        metadata: [
          {
            name: 'emailFrom'
            secretKeyRef: {
              name: 'email-secret'
              key: 'email-sender'
            }
          }
          {
            name: 'apiKey'
            secretKeyRef: {
              name: 'email-secret'
              key: 'api-key'
            }
          }
        ]
      }
    }
  }

  resource default 'Deployments' = {
    name: 'default'
    properties: {
      components: [
        {
          componentName: auditor.name
        }
        {
          componentName: processor.name
        }
        {
          componentName: viewer.name
        }
        {
          componentName: auditstore.name
        }
        {
          componentName: orderstore.name
        }
        {
          componentName: pubsub.name
        }
        {
          componentName: email.name
        }
      ]
    }
  }
}

resource auditstorecosmosaccount 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'auditstorecosmos-${uniqueString(resourceGroup().id)}'
  location: resourceGroup().location
  kind: 'GlobalDocumentDB'
  properties: {
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
       {
         locationName: resourceGroup().location
         failoverPriority: 0
         isZoneRedundant: false
       }
    ]
    databaseAccountOfferType: 'Standard'
  }
}

resource auditstorecosmosdb 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases@2020-04-01' = {
  name: '${auditstorecosmosaccount.name}/audits'
  properties: {
    resource: {
      id: 'audits'
    }
    options: {}
  }
}

resource audittorecosmoscontainer 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2020-04-01' = {
  name: '${auditstorecosmosdb.name}/record'
  properties: {
    resource: {
      id: 'record'
      partitionKey: {
        paths: [
          '/id'
        ]
        kind: 'Hash'
      }
    }
    options: {}
  }
}

resource orderstorecosmosaccount 'Microsoft.DocumentDB/databaseAccounts@2020-04-01' = {
  name: 'orderstorecosmos-${uniqueString(resourceGroup().id)}'
  location: resourceGroup().location
  kind: 'GlobalDocumentDB'
  properties: {
    consistencyPolicy: {
      defaultConsistencyLevel: 'Session'
    }
    locations: [
       {
         locationName: resourceGroup().location
         failoverPriority: 0
         isZoneRedundant: false
       }
    ]
    databaseAccountOfferType: 'Standard'
  }
}

resource orderstorecosmosdb 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases@2020-04-01' = {
  name: '${orderstorecosmosaccount.name}/orders'
  properties: {
    resource: {
      id: 'orders'
    }
    options: {}
  }
}

resource orderstorecosmoscontainer 'Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers@2020-04-01' = {
  name: '${orderstorecosmosdb.name}/record'
  properties: {
    resource: {
      id: 'record'
      partitionKey: {
        paths: [
          '/id'
        ]
        kind: 'Hash'
      }
    }
    options: {}
  }
}

resource sb 'microsoft.servicebus/namespaces@2017-04-01' = {
  name: 'sb-${uniqueString(resourceGroup().id)}'
  location: resourceGroup().location
  sku: {
    name: 'Standard'
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

resource blog 'microsoft.storage/storageAccounts/blobServices/containers@2019-06-01' = {
  name: '${stg.name}/default/workflows'
}
