param name string
param namespace string

module module 'module-dependency.bicep' = {
  name: 'module'
  params: {
    name: name
    namespace: namespace
  }
}

// Output the storage account ID
output envId string = module.outputs.envId
