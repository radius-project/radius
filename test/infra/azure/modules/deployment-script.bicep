// For more information, see https://docs.microsoft.com/en-us/azure/azure-resource-manager/bicep/deployment-script-bicep
@description('Specifies the name of the deployment script uri.')
param name string = 'BashScript' 

@description('Specifies the name of the AKS cluster.')
param clusterName string

@description('Specifies the resource group name')
param resourceGroupName string = resourceGroup().name

@description('Specifies the subscription id.')
param subscriptionId string = subscription().subscriptionId

@description('Specifies the tenant id.')
param tenantId string = subscription().tenantId

@description('Specifies the current datetime')
param utcValue string = utcNow()

@description('Specifies the location.')
param location string = resourceGroup().location

@description('Specifies the resource tags.')
param tags object

// Variables
var clusterAdminRoleDefinitionId = resourceId('Microsoft.Authorization/roleDefinitions', '0ab0b1a8-8aac-4efd-b8c2-3ee1fb270be8')

// Resources
resource aksCluster 'Microsoft.ContainerService/managedClusters@2023-05-01' existing = {
  name: clusterName
}

resource managedIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
  name: 'scriptManagedIdentity'
  location: location
  tags: tags
}

resource clusterAdminContributorRoleAssignment 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name:  guid(managedIdentity.id, aksCluster.id, clusterAdminRoleDefinitionId)
  scope: aksCluster
  properties: {
    roleDefinitionId: clusterAdminRoleDefinitionId
    principalId: managedIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

// Script
resource deploymentScript 'Microsoft.Resources/deploymentScripts@2020-10-01' = {
  name: name
  location: location
  kind: 'AzureCLI'
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${managedIdentity.id}': {}
    }
  }
  properties: {
    forceUpdateTag: utcValue
    azCliVersion: '2.50.0'
    timeout: 'PT30M'
    environmentVariables: [
      {
        name: 'clusterName'
        value: clusterName
      }
      {
        name: 'resourceGroupName'
        value: resourceGroupName
      }
      {
        name: 'subscriptionId'
        value: subscriptionId
      }
      {
        name: 'tenantId'
        value: tenantId
      }
    ]
    scriptContent: loadTextContent('bootstrap.sh')
    cleanupPreference: 'OnSuccess'
    retentionInterval: 'P1D'
  }
}

resource log 'Microsoft.Resources/deploymentScripts/logs@2020-10-01' existing = {
  parent: deploymentScript
  name: 'default'
}

output log string = log.properties.log
