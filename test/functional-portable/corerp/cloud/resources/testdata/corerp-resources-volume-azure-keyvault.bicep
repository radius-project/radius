extension radius

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the OIDC issuer URL for Azure workload identity.')
#disable-next-line no-hardcoded-env-urls
param oidcIssuer string = 'https://radiusoidc.blob.core.windows.net/kubeoidc/'

@description('Specifies the Azure resource group scope.')
param azureScope string

@description('Specifies the name of the test-owned Azure Key Vault.')
@minLength(3)
@maxLength(24)
param keyVaultName string

var applicationName = 'corerp-azure-keyvault-volume'
var containerName = 'volume-consumer'
var volumeName = 'keyvault-volume'
var secretName = 'csi-test-secret'
var secretValue = 'radius-csi-secret-value'

resource keyVault 'Microsoft.KeyVault/vaults@2023-07-01' = {
  name: keyVaultName
  location: resourceGroup().location
  properties: {
    enableRbacAuthorization: true
    enableSoftDelete: true
    softDeleteRetentionInDays: 7
    tenantId: subscription().tenantId
    sku: {
      family: 'A'
      name: 'standard'
    }
  }
}

resource keyVaultSecret 'Microsoft.KeyVault/vaults/secrets@2023-07-01' = {
  parent: keyVault
  name: secretName
  properties: {
    value: secretValue
  }
}

resource environment 'Applications.Core/environments@2023-10-01-preview' = {
  name: '${applicationName}-env'
  location: 'global'
  properties: {
    compute: {
      kind: 'kubernetes'
      namespace: '${applicationName}-env'
      identity: {
        kind: 'azure.com.workload'
        oidcIssuer: oidcIssuer
      }
    }
    providers: {
      azure: {
        scope: azureScope
      }
    }
  }
}

resource application 'Applications.Core/applications@2023-10-01-preview' = {
  name: applicationName
  location: 'global'
  properties: {
    environment: environment.id
    extensions: [
      {
        kind: 'kubernetesNamespace'
        namespace: applicationName
      }
    ]
  }
}

resource keyVaultVolume 'Applications.Core/volumes@2023-10-01-preview' = {
  name: volumeName
  location: 'global'
  properties: {
    application: application.id
    kind: 'azure.com.keyvault'
    resource: keyVault.id
    secrets: {
      mountedSecret: {
        name: secretName
        alias: 'secret-value'
      }
    }
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: containerName
  location: 'global'
  properties: {
    application: application.id
    container: {
      image: magpieimage
      command: [
        '/bin/sh'
        '-c'
      ]
      args: [
        '''
          set -eu
          test "$(cat /var/secrets/secret-value)" = "radius-csi-secret-value"
          echo csi-secret-mounted
          while true; do sleep 30; done
        '''
      ]
      volumes: {
        secrets: {
          kind: 'persistent'
          source: keyVaultVolume.id
          mountPath: '/var/secrets'
        }
      }
    }
  }
}

output keyVaultSecretID string = keyVaultSecret.id
