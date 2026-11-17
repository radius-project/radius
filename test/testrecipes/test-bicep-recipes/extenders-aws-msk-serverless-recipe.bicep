extension aws

@description('Name and alias of the MSK Serverless cluster. AWS assigns its ARN, so the test finds it by this name.')
param clusterName string

@description('Subnets for the cluster, in at least two Availability Zones.')
param subnetIds array

@description('Unix time the test started, used by the job that purges leaked test resources and by the CI role policy.')
param creationTimestamp string

// MSK Serverless can only be created and deleted. UCP reads its broker addresses with an extra
// GetBootstrapBrokers call, because AWS's schema doesn't return them.
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
    Tags: {
      RadiusCreationTimestamp: creationTimestamp
    }
  }
}

output result object = {
  values: {
    bootstrapBrokers: cluster.properties.BootstrapBrokerStringSaslIam
  }
}
