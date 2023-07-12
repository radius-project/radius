param grafanaPrincipalId string
param azureMonitorWorkspaceSubscriptionId string

@description('A new GUID used to identify the role assignment for Grafana')
param roleNameGuid string = newGuid()

resource roleAssignmentLocal 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: roleNameGuid
  properties: {
    roleDefinitionId: '/subscriptions/${azureMonitorWorkspaceSubscriptionId}/providers/Microsoft.Authorization/roleDefinitions/b0d8363b-8ddd-447d-831f-62ca05bff136'
    principalId: grafanaPrincipalId
  }
}
