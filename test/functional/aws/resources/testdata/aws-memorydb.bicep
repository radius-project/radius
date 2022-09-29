import aws as aws

resource testResource 'AWS.MemoryDB/Cluster@default' = {
name: 'my-test-cluster'
  properties: {
    NodeType: 'db.t4g.small' // https://aws.amazon.com/memorydb/pricing/
    ACLName: 'open-access'
    ClusterName: 'my-test-cluster'
  }
}
