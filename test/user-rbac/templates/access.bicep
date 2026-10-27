extension radius

@description('Trusted issuer returned by rad auth whoami.')
@minLength(1)
param identityIssuer string

@description('Resource group or application receiving the assignment.')
@minLength(1)
param assignmentScope string

@description('User receiving read access.')
@minLength(1)
param subject string = 'poc-outsider'

@description('Description used to exercise declarative updates.')
param roleDescription string = 'Managed by Bicep'

@description('Explicit permissions granted by the custom role.')
param permissions array = [
  'Radius.Core/applications/read'
]

resource customReader 'Radius.Core/roleDefinitions@2025-08-01-preview' = {
  name: 'poc-template-reader'
  properties: {
    displayName: 'Template Reader'
    description: roleDescription
    permissions: permissions
  }
}

resource readerAssignment 'Radius.Core/roleAssignments@2025-08-01-preview' = {
  name: guid(assignmentScope, customReader.id, identityIssuer, subject)
  scope: assignmentScope
  properties: {
    roleDefinitionId: customReader.id
    principal: {
      type: 'user'
      issuer: identityIssuer
      subject: subject
    }
  }
}

output roleId string = customReader.id
output assignmentId string = readerAssignment.id
