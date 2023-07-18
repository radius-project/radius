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


@description('Specifies the data collection rule resource id.')
param dataCollectionRuleId string

@description('Specifies the data collection rule resource association name.')
param dcraName string

@description('Specifies the AKS cluster name.')
param clusterName string

@description('Specifies the AKS cluster resource location.')
param clusterLocation string

#disable-next-line BCP174 // This warning is a false positive as dcra is already 'scope'-ed to the resource group in main template
resource dataCollectionRuleAssociations 'Microsoft.ContainerService/managedClusters/providers/dataCollectionRuleAssociations@2022-06-01' = {
  name: '${clusterName}/microsoft.insights/${dcraName}'
  location: clusterLocation
  properties: {
    description: 'Association of data collection rule. Deleting this association will break the data collection for this AKS Cluster.'
    dataCollectionRuleId: dataCollectionRuleId
  }
}
