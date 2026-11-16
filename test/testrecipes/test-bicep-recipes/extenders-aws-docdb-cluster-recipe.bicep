extension aws

@description('Identifier and alias of the DocumentDB cluster.')
param clusterIdentifier string

@description('DB subnet group for the cluster.')
param subnetGroupName string

@description('Master user password. AWS never returns it, so UCP compares it by fingerprint.')
@secure()
param masterUserPassword string

@description('Unix time the test started, used by the job that purges leaked test resources.')
param creationTimestamp string

// Cloud Control can't create a DocumentDB cluster, so UCP's native handler calls the DocumentDB API. The cluster has no
// instances, which keeps the test fast and cheap.
resource cluster 'AWS.DocDB/DBCluster@default' = {
  alias: clusterIdentifier
  properties: {
    DBClusterIdentifier: clusterIdentifier
    DBSubnetGroupName: subnetGroupName
    MasterUsername: 'radiustest'
    MasterUserPassword: masterUserPassword
    Tags: [
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}

output result object = {
  values: {
    endpoint: cluster.properties.Endpoint
  }
}
