@description('The name of the AKS cluster')
param clusterName string

@description('ACR registry name')
param registryName string

resource aks 'Microsoft.ContainerService/managedClusters@2021-08-01' existing = {
  name: clusterName
}

resource registry 'Microsoft.ContainerRegistry/registries@2021-06-01-preview' existing = {
  name: registryName
}

resource acrPullRole 'Microsoft.Authorization/roleDefinitions@2018-01-01-preview' existing = {
  name: '7f951dda-4ed3-4680-a7ca-43fe172d538d' // YUP. Role definition names are guids.
}

resource kubelet_roleassignment 'Microsoft.Authorization/roleAssignments@2020-10-01-preview' = {
  name: guid('rad-kubelet-', clusterName, resourceGroup().id) // YUP. Role assignment names are guids.
  scope: registry
  properties: {
    principalId: aks.properties.identityProfile.kubeletidentity.objectId
    principalType: 'ServicePrincipal'
    roleDefinitionId: acrPullRole.id
  }
}
