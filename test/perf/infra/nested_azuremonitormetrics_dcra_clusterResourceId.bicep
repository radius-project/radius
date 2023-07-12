param resourceId_Microsoft_Insights_dataCollectionRules_variables_dcrName string
param variables_clusterName string
param variables_dcraName string
param clusterLocation string

#disable-next-line BCP174 // This warning is a false positive as dcra is already 'scope'-ed to the resource group in main template
resource variables_clusterName_microsoft_insights_variables_dcra 'Microsoft.ContainerService/managedClusters/providers/dataCollectionRuleAssociations@2022-06-01' = {
  name: '${variables_clusterName}/microsoft.insights/${variables_dcraName}'
  location: clusterLocation
  properties: {
    description: 'Association of data collection rule. Deleting this association will break the data collection for this AKS Cluster.'
    dataCollectionRuleId: resourceId_Microsoft_Insights_dataCollectionRules_variables_dcrName
  }
}
