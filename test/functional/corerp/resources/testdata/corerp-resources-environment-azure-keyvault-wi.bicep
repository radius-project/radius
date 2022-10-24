// This is the template to create environment with workload identity
//
// 1. Create User assigned managed identity and Keyvault resource
// 2. Assign User assigned managed identity to Keyvault resource as Keyvault admin role.
// 3. Create Radius Environment resource with workload identity for the Keyvault created by step 1.
// 4. You can use the Identity in a resource (for now we only support Volume)

import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the environment for resources.')
param oidcIssuer string = 'https://radiusworkload-test'

@description('Specifies the value of the secret that you want to create.')
@secure()
param mySecretValue string = newGuid()

@description('Specifies the value of tenantId.')
param keyvaultTenantID string = subscription().tenantId

@description('Specifies the value of keyvault admin role.')
param keyvaultAdminRoleDefinitionId string = '/providers/Microsoft.Authorization/roleDefinitions/00482a5a-887f-4fb3-b363-3b7fe8e74483'

resource env 'Applications.Core/environments@2022-03-15-privatepreview' = {
  name: 'corerp-resources-env-with-identity-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'default'
      identity: {
        kind: 'azure.com.workload'
        resource: userAssignedIdentity.id
        oidcIssuer: oidcIssuer
      }
    }
  }
}

resource userAssignedIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2022-01-31-preview' = {
  name: 'kv-env-mi'
  location: location
}

resource keyVault 'Microsoft.KeyVault/vaults@2022-07-01' = {
  name: 'kv-env-vault'
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
  name: guid(keyVault.id, userAssignedIdentity.id, keyvaultAdminRoleDefinitionId)
  properties: {
    roleDefinitionId: keyvaultAdminRoleDefinitionId
    principalId: userAssignedIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

resource mySecret 'Microsoft.KeyVault/vaults/secrets@2022-07-01' = {
  parent: keyVault
  name: 'mysecret'
  properties: {
    value: mySecretValue
    attributes: {
      enabled: true
    }
  }
}
