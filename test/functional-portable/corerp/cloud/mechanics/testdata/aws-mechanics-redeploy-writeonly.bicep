extension aws

@description('Name and alias of the key pair.')
param keyPairName string

@description('Unix time the test started, used by the job that purges leaked test resources.')
param creationTimestamp string

// KeyFormat is write-only and create-only: AWS never returns it and it cannot change after create.
resource keyPair 'AWS.EC2/KeyPair@default' = {
  alias: keyPairName
  properties: {
    KeyName: keyPairName
    KeyFormat: 'pem'
    Tags: [
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}
