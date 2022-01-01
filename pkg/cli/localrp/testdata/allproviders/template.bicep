import kubernetes from kubernetes

resource storageNew 'Microsoft.Storage/storageAccounts@2021-06-01' = {
  name: 'storage-new'
  location: 'westus2'
  sku: {
    name: 'Premium_LRS'
  }
  kind: 'BlobStorage'
}

resource storageExisting 'Microsoft.Storage/storageAccounts@2021-06-01' existing = {
  name: 'storage-existing'
}

resource appNew 'radius.dev/Application@v1alpha3' = {
  name: 'app-new'
}

resource appExisting 'radius.dev/Application@v1alpha3' existing = {
  name: 'app-existing'
}

resource secretNew 'kubernetes.core/Secret@v1' = {
  metadata: {
    name: 'secretNew'
  }
  stringData: {
    storageNew: storageNew.sku.name
    storageExisting: storageExisting.sku.name
    secretExising: base64ToString(secretExisting.data.coolsecret)
  }
}

resource secretExisting 'kubernetes.core/Secret@v1' existing = {
  metadata: {
    name: 'secretExisting'
  }
}
