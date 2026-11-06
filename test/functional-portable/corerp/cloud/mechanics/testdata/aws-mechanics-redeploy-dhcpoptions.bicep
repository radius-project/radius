extension aws

@description('Alias of the DHCP option set, also set as its Name tag so the test can find it in AWS.')
param dhcpOptionsName string

@description('Unix time the test started, used by the job that purges leaked test resources.')
param creationTimestamp string

// AWS assigns the identifier of a DHCP option set, so the template sets no name and UCP finds the set by its alias.
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
