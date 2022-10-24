// This is the template to create an environment resource with system assigned managed identity.
//
// 1. Enable system assigned managed identity for AKS nodes - https://learn.microsoft.com/en-in/azure/aks/csi-secrets-store-identity-access#use-a-system-assigned-managed-identity
// 2. Make a note of principal id of system assigned managed identity created by step 1 and set this to the value of systemIdentityPrincipalId param.
// 3. Create Keyvault resource and assign system assigned managed identity to this resource as Keyvault admin role.
// 4. Create Radius Environment resource with the keyvault created by step 3.
// 5. You can use the Identity in a resource (for now we only support Volume)

import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the value of the secret that you want to create.')
@secure()
param mySecretValue string = newGuid()

@description('Specifies the value of tenantId.')
param keyvaultTenantID string = subscription().tenantId

@description('Specifies the value of keyvault admin role.')
param keyvaultAdminRoleDefinitionId string = '/providers/Microsoft.Authorization/roleDefinitions/00482a5a-887f-4fb3-b363-3b7fe8e74483'

@description('Specifies the principal ID of System assigned managed identity of VMSS. See this - https://learn.microsoft.com/en-in/azure/aks/csi-secrets-store-identity-access#use-a-system-assigned-managed-identity')
param systemIdentityPrincipalId string

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-env-azkv'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'default'
      identity: {
        kind: 'azure.com.systemassigned'
      }
    }
  }
}

resource keyVault 'Microsoft.KeyVault/vaults@2022-07-01' = {
  name: 'test-kv'
  location: location
  tags: {
    radiustest: 'corerp-resources-key-vault'
  }
  properties: {
    enabledForTemplateDeployment: true
    tenantId: keyvaultTenantID
    enableRbacAuthorization: true
    sku: {
      name: 'standard'
      family: 'A'
    }
  }
}

resource roleAssignment 'Microsoft.Authorization/roleAssignments@2020-04-01-preview' = {
  scope: keyVault
  name: guid(keyVault.id, systemIdentityPrincipalId, keyvaultAdminRoleDefinitionId)
  properties: {
    roleDefinitionId: keyvaultAdminRoleDefinitionId
    principalId: systemIdentityPrincipalId
    principalType: 'ServicePrincipal'
  }
}

resource secret 'Microsoft.KeyVault/vaults/secrets@2022-07-01' = {
  parent: keyVault
  name: 'mysecret'
  properties: {
    value: mySecretValue
    attributes: {
      enabled: true
    }
  }
}
