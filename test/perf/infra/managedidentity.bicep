// Parameters
@description('Specifies the name of the user-defined managed identity.')
param managedIdentityName string

@description('Specifies the name of the existing virtual network.')
param virtualNetworkName string

@description('Specifies the name of the subnet hosting the worker nodes of the default system agent pool of the AKS cluster.')
param systemAgentPoolSubnetName string = 'SystemSubnet'

@description('Specifies the name of the subnet hosting the worker nodes of the user agent pool of the AKS cluster.')
param userAgentPoolSubnetName string = 'UserSubnet'

@description('Specifies the name of the subnet hosting the pods running in the AKS cluster.')
param podSubnetName string = 'PodSubnet'

@description('Specifies the name of the subnet delegated to the API server when configuring the AKS cluster to use API server VNET integration.')
param apiServerSubnetName string = 'ApiServerSubnet'

@description('Specifies the location.')
param location string = resourceGroup().location

@description('Specifies the resource tags.')
param tags object

// Variables
var networkContributorRoleDefinitionId = resourceId('Microsoft.Authorization/roleDefinitions', '4d97b98b-1d4f-4787-a291-c67834d212e7')

// Resources
resource managedIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2021-09-30-preview' = {
  name: managedIdentityName
  location: location
  tags: tags
}

resource virtualNetwork 'Microsoft.Network/virtualNetworks@2021-08-01' existing =  {
  name: virtualNetworkName
}

resource systemAgentPoolSubnet 'Microsoft.Network/virtualNetworks/subnets@2021-08-01' existing = {
  parent: virtualNetwork
  name: systemAgentPoolSubnetName
}

resource userAgentPoolSubnet 'Microsoft.Network/virtualNetworks/subnets@2021-08-01' existing = {
  parent: virtualNetwork
  name: userAgentPoolSubnetName
}

resource podSubnet 'Microsoft.Network/virtualNetworks/subnets@2021-08-01' existing = {
  parent: virtualNetwork
  name: podSubnetName
}

resource apiServerSubnet 'Microsoft.Network/virtualNetworks/subnets@2021-08-01' existing = {
  parent: virtualNetwork
  name: apiServerSubnetName
}

resource systemAgentPoolSubnetNetworkContributorRoleAssignment 'Microsoft.Authorization/roleAssignments@2020-10-01-preview' = {
  name:  guid(managedIdentity.id, systemAgentPoolSubnet.id, networkContributorRoleDefinitionId)
  scope: systemAgentPoolSubnet
  properties: {
    roleDefinitionId: networkContributorRoleDefinitionId
    principalId: managedIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

resource userAgentPoolSubnetNetworkContributorRoleAssignment 'Microsoft.Authorization/roleAssignments@2020-10-01-preview' = {
  name:  guid(managedIdentity.id, userAgentPoolSubnet.id, networkContributorRoleDefinitionId)
  scope: userAgentPoolSubnet
  properties: {
    roleDefinitionId: networkContributorRoleDefinitionId
    principalId: managedIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

resource podSubnetNetworkContributorRoleAssignment 'Microsoft.Authorization/roleAssignments@2020-10-01-preview' = {
  name:  guid(managedIdentity.id, podSubnet.id, networkContributorRoleDefinitionId)
  scope: podSubnet
  properties: {
    roleDefinitionId: networkContributorRoleDefinitionId
    principalId: managedIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

resource apiServerSubnetNetworkContributorRoleAssignment 'Microsoft.Authorization/roleAssignments@2020-10-01-preview' = {
  name:  guid(managedIdentity.id, apiServerSubnet.id, networkContributorRoleDefinitionId)
  scope: apiServerSubnet
  properties: {
    roleDefinitionId: networkContributorRoleDefinitionId
    principalId: managedIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

// Outputs
output id string = managedIdentity.id
output name string = managedIdentity.name
