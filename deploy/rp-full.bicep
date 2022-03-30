@description('The name of the AKS cluster')
param clusterName string = 'radius-aks-${uniqueString(resourceGroup().id)}'

@description('The name of the managed identity assigned to the AKS cluster control-plane')
param aksIdentityName string = 'radius-aks-id-${uniqueString(resourceGroup().id)}'

@description('The name of the managed identity assigned to the Radius RP')
param rpIdentityName string = 'radius-rp-id-${uniqueString(resourceGroup().id)}'

@description('The tags to apply to each resource')
param resourceTags object = {
  'rad-environment': true
}

@description('The ARM resource ID of the log analytics workspace where the Radius Resource Provider logs will be redirected to.')
param logAnalyticsWorkspaceID string = ''

@description('Optional ACR registry name')
param registryName string = ''

var addonsWithLogAnalytics = {
  azureKeyvaultSecretsProvider: {
    config: {
      enableSecretRotation: 'true'
    }
    enabled: true
  }
  omsagent: {
    config: {
      logAnalyticsWorkspaceResourceID: logAnalyticsWorkspaceID
    }
    enabled: true
  }
}

var addonsWithoutLogAnalytics = {
  azureKeyvaultSecretsProvider: {
    config: {
      enableSecretRotation: 'true'
    }
    enabled: true
  }
}

resource aks 'Microsoft.ContainerService/managedClusters@2021-08-01' = {
  name: clusterName
  location: resourceGroup().location
  tags: resourceTags
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities:{
      '${aks_identity.id}': {}
    }
  }
  properties: {
    addonProfiles: empty(logAnalyticsWorkspaceID) ? addonsWithoutLogAnalytics : addonsWithLogAnalytics
    agentPoolProfiles: [
      {
          enableAutoScaling: true
          count: 2 // YES this IS required even though we're using autoscaling.
          minCount: 2
          maxCount: 5
          mode: 'System'
          name: 'agentpool'
          osDiskSizeGB: 0
          vmSize: 'Standard_B2ms'
      }
    ]
    dnsPrefix: clusterName
    enableRBAC: true
    networkProfile: {
      networkPlugin: 'azure'
    }
    podIdentityProfile: {
      enabled: true
      userAssignedIdentities: [
        {
          identity: {
            clientId: rp_identity.properties.clientId
            objectId: rp_identity.properties.principalId
            resourceId: rp_identity.id
          }
          name: 'radius'
          namespace: 'radius-system'
        }
      ]
    }
  }
}

module registry 'rp-registry.bicep' =  if (!empty(registryName)) {
  name: 'rad-registry-${uniqueString(resourceGroup().id)}'
  params: {
    clusterName: aks.name
    registryName: registryName
  }
}

resource aks_identity 'Microsoft.ManagedIdentity/userAssignedIdentities@2018-11-30' = {
  name: aksIdentityName
  location: resourceGroup().location
  tags: resourceTags
}

resource rp_identity 'Microsoft.ManagedIdentity/userAssignedIdentities@2018-11-30' = {
  name: rpIdentityName
  location: resourceGroup().location
  tags: resourceTags
}

resource aks_roleassignment 'Microsoft.Authorization/roleAssignments@2020-10-01-preview' = {
  name: guid('rad-aks-', clusterName, resourceGroup().id) // YUP. Role assignment names are guids.
  properties: {
    principalId: aks_identity.properties.principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: managedIdentityOperatorRole.id
  }
}

resource rp_roleassignment 'Microsoft.Authorization/roleAssignments@2020-10-01-preview' = {
  name: guid('rad-rp-', clusterName, resourceGroup().id) // YUP. Role assignment names are guids.
  properties: {
    principalId: rp_identity.properties.principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: ownerRole.id
  }
}

resource managedIdentityOperatorRole 'Microsoft.Authorization/roleDefinitions@2018-01-01-preview' existing = {
  name: 'f1a07417-d97a-45cb-824c-7a7467783830' // YUP. Role definition names are guids.
}

resource ownerRole 'Microsoft.Authorization/roleDefinitions@2018-01-01-preview' existing = {
  name: '8e3af657-a8ff-443c-a75c-2fe8c4bcb635' // YUP. Role definition names are guids.
}

output clusterName string = aks.name
