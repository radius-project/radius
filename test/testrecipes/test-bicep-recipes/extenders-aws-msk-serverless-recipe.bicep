extension aws

@description('Name and alias of the MSK Serverless cluster. AWS assigns its ARN, so the test finds it by this name.')
param clusterName string

@description('Subnets for the cluster, in at least two Availability Zones.')
param subnetIds array

// MSK Serverless can only be created and deleted. UCP reads its broker addresses with an extra
// GetBootstrapBrokers call, because AWS's schema doesn't return them. The cluster has no tags: the Bicep type of
// its Tags map accepts no keys. The test deletes it by name instead.
resource cluster 'AWS.MSK/ServerlessCluster@default' = {
  alias: clusterName
  properties: {
    ClusterName: clusterName
    ClientAuthentication: {
      Sasl: {
        Iam: {
          Enabled: true
        }
      }
    }
    VpcConfigs: [
      {
        SubnetIds: subnetIds
      }
    ]
  }
}

output result object = {
  values: {
    bootstrapBrokers: cluster.properties.BootstrapBrokerStringSaslIam
  }
}
