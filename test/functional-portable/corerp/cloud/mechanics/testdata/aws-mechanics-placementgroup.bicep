extension aws

@description('Alias of the placement group and value of its Name tag. AWS generates the group name, so the test finds the group by this tag.')
param name string

@description('Placement strategy. AWS can create and delete a placement group but not change it.')
param strategy string = 'spread'

@description('Unix time the test started, used by the job that purges leaked test resources.')
param creationTimestamp string

// AWS::EC2::PlacementGroup can only be created and deleted, so Cloud Control has no update for it.
resource placementGroup 'AWS.EC2/PlacementGroup@default' = {
  alias: name
  properties: {
    Strategy: strategy
    Tags: [
      {
        Key: 'Name'
        Value: name
      }
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}
