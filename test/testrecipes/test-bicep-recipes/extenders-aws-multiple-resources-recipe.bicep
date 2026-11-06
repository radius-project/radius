extension aws

@description('Name and alias of the log group.')
param logGroupName string

@description('Alias of the DHCP option set, also set as its Name tag so the test can find it in AWS.')
param dhcpOptionsName string

@description('Unix time the test started, used by the job that purges leaked test resources.')
param creationTimestamp string

// A recipe with two AWS output resources. Deleting the Radius resource must delete both.
resource logGroup 'AWS.Logs/LogGroup@default' = {
  alias: logGroupName
  properties: {
    LogGroupName: logGroupName
    Tags: [
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}

// AWS assigns the identifier of a DHCP option set, so UCP deletes it by the identifier it recorded for the alias.
resource dhcpOptions 'AWS.EC2/DHCPOptions@default' = {
  alias: dhcpOptionsName
  properties: {
    DomainNameServers: [
      'AmazonProvidedDNS'
    ]
    Tags: [
      {
        Key: 'Name'
        Value: dhcpOptionsName
      }
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}
