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

@description('Specifies the AKS cluster resource Id.')
param clusterResourceId string

@description('Specifies the AKS cluster resource location.')
param clusterLocation string

@description('Specifies the azure monitor workspace resource id.')
param azureMonitorWorkspaceId string

@description('Specifies the azure monitor workspace resource location.')
param azureMonitorWorkspaceLocation string

@description('Specifies the resource tags.')
param tags object

var clusterSubscriptionId = split(clusterResourceId, '/')[2]
var clusterResourceGroup = split(clusterResourceId, '/')[4]
var clusterName = split(clusterResourceId, '/')[8]

var dceName = 'MSProm-${azureMonitorWorkspaceLocation}-${clusterName}'
var dcrName = 'MSProm-${azureMonitorWorkspaceLocation}-${clusterName}'
var dcraName = 'MSProm-${clusterLocation}-${clusterName}'

resource dce 'Microsoft.Insights/dataCollectionEndpoints@2022-06-01' = {
  name: dceName
  location: azureMonitorWorkspaceLocation
  kind: 'Linux'
  properties: {}
  tags: tags
}

resource dcr 'Microsoft.Insights/dataCollectionRules@2022-06-01' = {
  name: dcrName
  location: azureMonitorWorkspaceLocation
  kind: 'Linux'
  properties: {
    dataCollectionEndpointId: dce.id
    dataFlows: [
      {
        destinations: [
          'MonitoringAccount1'
        ]
        streams: [
          'Microsoft-PrometheusMetrics'
        ]
      }
    ]
    dataSources: {
      prometheusForwarder: [
        {
          name: 'PrometheusDataSource'
          streams: [
            'Microsoft-PrometheusMetrics'
          ]
          labelIncludeFilter: {
          }
        }
      ]
    }
    description: 'DCR for Azure Monitor Metrics Profile (Managed Prometheus)'
    destinations: {
      monitoringAccounts: [
        {
          accountResourceId: azureMonitorWorkspaceId
          name: 'MonitoringAccount1'
        }
      ]
    }
  }
  tags: tags
}

module azureMonitorMetricsDcraClusterResourceId './datacollection-dcra.bicep' = {
  name: 'azuremonitormetrics-dcra-${uniqueString(clusterResourceId)}'
  scope: resourceGroup(clusterSubscriptionId, clusterResourceGroup)
  params: {
    dataCollectionRuleId: dcr.id
    clusterName: clusterName
    dcraName: dcraName
    clusterLocation: clusterLocation
  }
  dependsOn: [
    dce
  ]
}

// Output
output dcrId string = dcr.id
