// Parameters ------------------------------------------------------
param serverName string = uniqueString('sql', resourceGroup().id)
param location string = resourceGroup().location
param skuName string = 'Standard'
param skuTier string = 'Standard'
param adminLogin string
@secure()
param adminPassword string

// Azure Bicep resources ------------------------------------------------------
resource sql 'Microsoft.Sql/servers@2019-06-01-preview' = {
  name: serverName
  location: location
  properties: {
    administratorLogin: adminLogin
    administratorLoginPassword: adminPassword
  }

  resource identity 'databases' = {
    name: 'IdentityDb'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }

  resource catalog 'databases' = {
    name: 'CatalogDb'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }

  resource ordering 'databases' = {
    name: 'OrderingDb'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }

  resource webhooks 'databases' = {
    name: 'WebhooksDb'
    location: location
    sku: {
      name: skuName
      tier: skuTier
    }
  }
}

// Radius resources ------------------------------------------------------
resource eshop 'radius.dev/Application@v1alpha3' = {
  name: 'eshop'

  resource sqlIdentity 'microsoft.com.SQLComponent' = {
    name: 'IdentityDb'
    properties: {
      resource: sql::identity.id
    }
  }

  resource sqlCatalog 'microsoft.com.SQLComponent' = {
    name: 'CatalogDb'
    properties: {
      resource: sql::catalog.id
    }
  }

  resource sqlOrdering 'microsoft.com.SQLComponent' = {
    name: 'OrderingDb'
    properties: {
      resource: sql::ordering.id
    }
  }

  resource sqlWebhooks 'microsoft.com.SQLComponent' = {
    name: 'WebhooksDb'
    properties: {
      resource: sql::webhooks.id
    }
  }

}
