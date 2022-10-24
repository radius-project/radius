// This is the template to create keyvault volume with system assigned managed identity.
//
// 1. Enable system assigned managed identity for AKS nodes - https://learn.microsoft.com/en-in/azure/aks/csi-secrets-store-identity-access#use-a-system-assigned-managed-identity
// 2. Make a note of principal id of system assigned managed identity created by step 1 and set this to the value of systemIdentityPrincipalId param.
// 3. Create Keyvault resource and assign system assigned managed identity to this resource as Keyvault admin role.
// 4. Create Radius Volume resource for the keyvault created by step 3.
// 5. Associate Radius volume to Container resource.

import radius as radius

@description('Specifies the location for resources.')
param location string = 'global'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the port of the container resource.')
param port int = 3000

@description('Specifies the environment for resources.')
param environment string

@description('Specifies the value of the secret that you want to create.')
@secure()
param mySecretValue string = newGuid()

@description('Specifies the value of tenantId.')
param keyvaultTenantID string = subscription().tenantId

@description('Specifies the value of keyvault admin role.')
param keyvaultAdminRoleDefinitionId string = '/providers/Microsoft.Authorization/roleDefinitions/00482a5a-887f-4fb3-b363-3b7fe8e74483'

@description('Specifies the principal ID of System assigned managed identity of VMSS. See this - https://learn.microsoft.com/en-in/azure/aks/csi-secrets-store-identity-access#use-a-system-assigned-managed-identity')
param systemIdentityPrincipalId string

resource app 'Applications.Core/applications@2022-03-15-privatepreview' = {
  name: 'corerp-resources-volume-azkv'
  location: location
  properties: {
    environment: environment
  }
}

resource keyvaultVolume 'Applications.Core/volumes@2022-03-15-privatepreview' = {
  name: 'volume-azkv'
  location: location
  properties: {
    application: app.id
    kind: 'azure.com.keyvault'
    identity: {
      kind: 'azure.com.systemassigned'
    }
    resource: azTestKeyvault.id
    secrets: {
      mysecret: {
        name: 'mysecret'
      }
    }
  }
}

resource keyvaultVolContainer 'Applications.Core/containers@2022-03-15-privatepreview' = {
  name: 'volume-azkv-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      ports: {
        web: {
          containerPort: port
        }
      }
      volumes: {
        volkv: {
            kind: 'persistent'
            source: keyvaultVolume.id
            mountPath: '/var/secrets'
        }
      }
    }
  }
}

// Prepare Azure resources - keyvault and role assignment.
resource azTestKeyvault 'Microsoft.KeyVault/vaults@2022-07-01' = {
  name: 'kv-volume-1'
  location: location
  tags: {
    radiustest: 'corerp-resources-key-vault'
  }
  properties: {
    enabledForTemplateDeployment: true
    tenantId: keyvaultTenantID
    enableRbacAuthorization:true
    sku: {
      name: 'standard'
      family: 'A'
    }
  }
}

resource roleAssignment 'Microsoft.Authorization/roleAssignments@2020-04-01-preview' = {
  scope: azTestKeyvault
  name: guid(azTestKeyvault.id, systemIdentityPrincipalId, keyvaultAdminRoleDefinitionId)
  properties: {
    roleDefinitionId: keyvaultAdminRoleDefinitionId
    principalId: systemIdentityPrincipalId
    principalType: 'ServicePrincipal'
  }
}

resource mySecret 'Microsoft.KeyVault/vaults/secrets@2022-07-01' = {
  parent: azTestKeyvault
  name: 'mysecret'
  properties: {
    value: mySecretValue
    attributes:{
      enabled:true
    }
  }
}
