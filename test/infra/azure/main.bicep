/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

@description('Specifies the prefix for resource names deployed in this template.')
param prefix string = uniqueString(resourceGroup().id)

@description('Specifies the location where to deploy the resources. Default is the resource group location.')
param location string = resourceGroup().location

@description('Specifies the name of log anlaytics workspace. Default is {prefix}-workspace.')
param logAnalyticsWorkspaceName string = '${prefix}-workspace'

@description('Specifies the location of log anlaytics workspace. Default is the resource group location.')
param logAnalyticsWorkspaceLocation string = resourceGroup().location

@description('Specifies the location of azure monitor workspace. Default is {prefix}-azm-workspace.')
param azureMonitorWorkspaceName string = '${prefix}-azm-workspace'

@allowed([
  'eastus2euap'
  'centraluseuap'
  'centralus'
  'eastus'
  'eastus2'
  'northeurope'
  'southcentralus'
  'southeastasia'
  'uksouth'
  'westeurope'
  'westus'
  'westus2'
])
@description('Specifies the location of azure monitor workspace. Default is westus2')
param azureMonitorWorkspaceLocation string = 'westus2'

@description('Specifies the name of aks cluster. Default is {prefix}-aks.')
param aksClusterName string = '${prefix}-aks'

@description('Specifies the object id to assign Grafana administrator role. Can be the object id of AzureAD user or group.')
param grafanaAdminObjectId string

@description('Specifies the name of Grafana dashboard. Default is {prefix}-dashboard.')
param grafanaDashboardName string = '${prefix}-dashboard'

param defaultTags object = {
  radius: 'infra'
}

// Deploy Log Analytics Workspace for log.
module logAnalyticsWorkspace './modules/loganalytics-workspace.bicep' = {
  name: logAnalyticsWorkspaceName
  params: {
    name: logAnalyticsWorkspaceName
    location: logAnalyticsWorkspaceLocation
    sku: 'PerGB2018'
    retentionInDays: 30
    tags: defaultTags
  }
}

// Deploy Azure Monitor Workspace for metrics.
resource azureMonitorWorkspace 'microsoft.monitor/accounts@2023-04-03' = {
  name: azureMonitorWorkspaceName
  location: azureMonitorWorkspaceLocation
  properties: {}
}

// Deploy AKS cluster with OIDC Issuer profile and Dapr.
module aksCluster './modules/akscluster.bicep' = {
  name: aksClusterName
  params:{
    name: aksClusterName
    location: location
    kubernetesVersion: '1.26.3'
    logAnalyticsWorkspaceId: logAnalyticsWorkspace.outputs.id
    systemAgentPoolName: 'agentpool'
    systemAgentPoolVmSize: 'Standard_D4as_v5'
    systemAgentPoolAvailabilityZones: []
    systemAgentPoolOsDiskType: 'Managed'
    userAgentPoolName: 'userpool'
    userAgentPoolVmSize: 'Standard_D4as_v5'
    userAgentPoolAvailabilityZones: []
    userAgentPoolOsDiskType: 'Managed'
    daprEnabled: true
    daprHaEnabled: false
    oidcIssuerProfileEnabled: true
    tags: defaultTags
  }
}

// Deploy Grafana dashboard.
module grafanaDashboard './modules/grafana.bicep' = {
  name: grafanaDashboardName
  params:{
    name: grafanaDashboardName
    location: location
    adminObjectId: grafanaAdminObjectId
    azureMonitorWorkspaceId: azureMonitorWorkspace.id
    clusterResourceId: aksCluster.outputs.id
    clusterLocation: aksCluster.outputs.location
    tags: defaultTags
  }
}

// Deploy data collection for metrics.
module dataCollection './modules/datacollection.bicep' = {
  name: 'dataCollection'
  params:{
    azureMonitorWorkspaceLocation: azureMonitorWorkspace.location
    azureMonitorWorkspaceId: azureMonitorWorkspace.id
    clusterResourceId: aksCluster.outputs.id
    clusterLocation: aksCluster.outputs.location
    tags: defaultTags
  }
  dependsOn: [
    grafanaDashboard
  ]
}

// Deploy alert rules using prometheus metrics.
module alertManagement './modules/alert-management.bicep' = {
  name: 'alertManagement'
  params:{
    azureMonitorWorkspaceLocation: azureMonitorWorkspace.location
    azureMonitorWorkspaceResourceId: azureMonitorWorkspace.id
    clusterResourceId: aksCluster.outputs.id
    tags: defaultTags
  }
  dependsOn: [
    dataCollection
  ]
}

// This is a workaround to get the AKS cluster resource created by aksCluster module
resource aks 'Microsoft.ContainerService/managedClusters@2023-05-01' existing = {
  name: aksCluster.name
}

// Deploy configmap for prometheus metrics.
module promConfigMap './modules/ama-metrics-setting-configmap.bicep' = {
  name: 'metrics-configmap'
  params: {
    kubeConfig: aks.listClusterAdminCredential().kubeconfigs[0].value
  }
  dependsOn: [
    aks, aksCluster, dataCollection, alertManagement
  ]
}

// Run deployment script to bootstrap the cluster for Radius.
module deploymentScript './modules/deployment-script.bicep' = {
  name: 'deploymentScript'
  params: {
    name: 'radiusBootStrap'
    clusterName: aksCluster.outputs.name
    resourceGroupName: resourceGroup().name
    subscriptionId: subscription().subscriptionId
    tenantId: subscription().tenantId
    location: location
    tags: defaultTags
  }
  dependsOn: [
    aksCluster
  ]
}
