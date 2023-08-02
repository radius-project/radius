/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

@description('Specifies the AKS cluster resource Id.')
param clusterResourceId string

@description('Specifies the AKS cluster resource location.')
param clusterLocation string

@description('Specifies the azure monitor workspace resource id.')
param logAnalyticsWorkspaceId string

@description('Specifies the log analytics workspace resource location.')
param logAnalyticsWorkspaceLocation string

@description('Specifies the resource tags.')
param tags object

var clusterSubscriptionId = split(clusterResourceId, '/')[2]
var clusterResourceGroup = split(clusterResourceId, '/')[4]
var clusterName = split(clusterResourceId, '/')[8]

var dcrName = 'MSCI-${logAnalyticsWorkspaceLocation}-${clusterName}'
var dcraName = 'MSCI-${clusterLocation}-${clusterName}'

resource dcr 'Microsoft.Insights/dataCollectionRules@2022-06-01' = {
  name: dcrName
  location: logAnalyticsWorkspaceLocation
  kind: 'Linux'
  properties: {
    dataSources: {
      extensions: [
          {
              name: 'ContainerInsightsExtension'
              streams: [
                  'Microsoft-ContainerInsights-Group-Default'
              ]
              extensionName: 'ContainerInsights'
              extensionSettings: {
                  dataCollectionSettings: {
                      interval: '1m'
                      namespaceFilteringMode: 'Off'
                      enableContainerLogV2: true
                  }
              }
          }
      ]
      syslog: []
    }
    destinations: {
        logAnalytics: [
            {
                workspaceResourceId: logAnalyticsWorkspaceId
                name: 'ciworkspace'
            }
        ]
    }
    dataFlows: [
        {
            streams: [
                'Microsoft-ContainerInsights-Group-Default'
            ]
            destinations: [
                'ciworkspace'
            ]
        }
    ]
  }
  tags: tags
}

module logAnalyticsDcraClusterResourceId './datacollection-dcra.bicep' = {
  name: 'loganalytics-dcra-${uniqueString(clusterResourceId)}'
  scope: resourceGroup(clusterSubscriptionId, clusterResourceGroup)
  params: {
    dataCollectionRuleId: dcr.id
    clusterName: clusterName
    dcraName: dcraName
    clusterLocation: clusterLocation
  }
}

// Output
output dcrId string = dcr.id
