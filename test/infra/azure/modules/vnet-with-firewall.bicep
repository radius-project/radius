@description('Virtual network name')
param virtualNetworkName string = 'vnet-${uniqueString(resourceGroup().id)}'

@description('Azure Firewall name')
param firewallName string = 'fw${uniqueString(resourceGroup().id)}'

@description('Number of public IP addresses for the Azure Firewall')
@minValue(1)
@maxValue(100)
param numberOfPublicIPAddresses int = 2

@description('Zone numbers e.g. 1,2,3.')
param availabilityZones array = []

@description('Location for all resources.')
param location string = resourceGroup().location

@description('Specifies the resource tags.')
param tags object = {
  'radapp.io': 'radius-infra'
}

param firewallPolicyName string = '${firewallName}-firewallPolicy'

// Starts with `10.10.0.0` (network address) and ends with `10.10.255.255` (broadcast address).
// [65536 - 2 = 65534] usable IP addresses for hosts like servers, virtual machines, etc.
var vnetAddressPrefix = '10.10.0.0/16'

// Starts with `10.10.0.0` (network address) and ends with `10.10.15.255` (broadcast address).
// [4096 - 2 = 4094] usable IP addresses for hosts.
var azureFirewallSubnetPrefix = '10.10.0.0/20'

// Starts with `10.10.16.0` (network address) and ends with `10.10.31.255` (broadcast address).
// [4096 - 2 = 4094] usable IP addresses for hosts.
var aksPoolsSubnetPrefix = '10.10.16.0/20'
var aksPoolsSubnetName = 'aks-pools-subnet'

var publicIPNamePrefix = 'publicIP'
var azurepublicIpname = publicIPNamePrefix

@description('Specify a Network Security Group name.')
param networkSecurityGroupName string = 'nsg-${uniqueString(resourceGroup().id)}'

@description('Specify a DDoS protection plan name.')
param ddosProtectionPlanName string = 'ddos-${uniqueString(resourceGroup().id)}'

var azureFirewallSubnetName = 'AzureFirewallSubnet'
var azureFirewallSubnetId = resourceId('Microsoft.Network/virtualNetworks/subnets', virtualNetworkName, azureFirewallSubnetName)
var azureFirewallPublicIpId = resourceId('Microsoft.Network/publicIPAddresses', publicIPNamePrefix)
var azureFirewallIpConfigurations = [for i in range(0, numberOfPublicIPAddresses): {
  name: 'IpConf${i}'
  properties: {
    subnet: ((i == 0) ? json('{"id": "${azureFirewallSubnetId}"}') : json('null'))
    publicIPAddress: {
      id: '${azureFirewallPublicIpId}${i + 1}'
    }
  }
}]

var vnetTags = union(tags, {
    displayName: virtualNetworkName
  }
)

resource networkSecurityGroup 'Microsoft.Network/networkSecurityGroups@2022-01-01' = {
  name: networkSecurityGroupName
  location: location
  tags: tags
}

resource ddosProtectionPlan 'Microsoft.Network/ddosProtectionPlans@2021-05-01' = {
  name: ddosProtectionPlanName
  location: location
}

resource vnet 'Microsoft.Network/virtualNetworks@2022-01-01' = {
  name: virtualNetworkName
  location: location
  tags: vnetTags
  properties: {
    addressSpace: {
      addressPrefixes: [
        vnetAddressPrefix
      ]
    }
    subnets: [
      {
        name: aksPoolsSubnetName
        properties: {
          addressPrefix: aksPoolsSubnetPrefix
          networkSecurityGroup: {
            id: networkSecurityGroup.id
          }
        }
      }
      {
        name: azureFirewallSubnetName
        properties: {
          addressPrefix: azureFirewallSubnetPrefix
        }
      }
    ]
    enableDdosProtection: true
    ddosProtectionPlan: {
      id: ddosProtectionPlan.id
    }
  }
}

resource publicIpAddress 'Microsoft.Network/publicIPAddresses@2022-01-01' = [for i in range(0, numberOfPublicIPAddresses): {
  name: '${azurepublicIpname}${i + 1}'
  location: location
  tags: tags
  sku: {
    name: 'Standard'
  }
  properties: {
    publicIPAllocationMethod: 'Static'
    publicIPAddressVersion: 'IPv4'
  }
}]

resource firewallPolicy 'Microsoft.Network/firewallPolicies@2022-01-01' = {
  name: firewallPolicyName
  location: location
  tags: tags
  properties: {
    sku: {
      tier: 'Premium'
    }
    threatIntelMode: 'Alert'
  }
}

resource firewall 'Microsoft.Network/azureFirewalls@2021-03-01' = {
  name: firewallName
  location: location
  tags: tags
  zones: ((length(availabilityZones) == 0) ? null : availabilityZones)
  dependsOn: [
    vnet
    publicIpAddress
  ]
  properties: {
    sku: {
      tier: 'Premium'
    }
    ipConfigurations: azureFirewallIpConfigurations
    firewallPolicy: {
      id: firewallPolicy.id
    }
  }
}

// Outputs
output aksPoolsSubnetID string = vnet.properties.subnets[0].id
output azureFirewallSubnetID string = vnet.properties.subnets[1].id
output networkSecurityGroup string = networkSecurityGroup.id
