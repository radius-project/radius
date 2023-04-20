import radius as radius

@description('Specifies the location for resources.')
param location string = 'eastus'

@description('Specifies the image of the container resource.')
param magpieimage string

@description('Specifies the oidc issuer URL.')
#disable-next-line no-hardcoded-env-urls
param oidcIssuer string = 'https://radiusoidc.blob.core.windows.net/kubeoidc/'

resource env 'Applications.Core/environments@2023-04-15-preview' = {
  name: 'azstorage-workload-env'
  location: location
  properties: {
    compute: {
      kind: 'kubernetes'
      resourceId: 'self'
      namespace: 'azstorage-workload-env'
      identity: {
        kind: 'azure.com.workload'
        oidcIssuer: oidcIssuer
      }
    }
    providers: {
      azure: {
        scope: resourceGroup().id
      }
    }
  }
}

resource app 'Applications.Core/applications@2023-04-15-preview' = {
  name: 'corerp-resources-container-workload'
  location: location
  properties: {
    environment: env.id
    extensions: [
      {
          kind: 'kubernetesNamespace'
          namespace: 'azstorage-workload-app'
      }
    ]
  }
}

resource container 'Applications.Core/containers@2023-04-15-preview' = {
  name: 'azstorage-ctnr'
  location: location
  properties: {
    application: app.id
    container: {
      image: magpieimage
      env: {
        CONNECTION_STORAGE_ACCOUNTNAME: storageAccount.name
      }
      readinessProbe:{
        kind:'httpGet'
        containerPort:3000
        path: '/healthz'
      }
    }
    connections: {
      storage: {
        source: storageAccount.id
        iam: {
          kind: 'azure'
          roles: [
            'Storage Blob Data Contributor'
          ]
        }
      }
    }
  }
}

resource storageAccount 'Microsoft.Storage/storageAccounts@2021-09-01' = {
  name: 'sawi${uniqueString(resourceGroup().id, deployment().name)}'
  location: location
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
  properties: {
    accessTier: 'Hot'
  }
}
