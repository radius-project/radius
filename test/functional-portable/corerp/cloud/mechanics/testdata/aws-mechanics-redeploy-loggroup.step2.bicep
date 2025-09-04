extension aws

param creationTimestamp string
param logGroupName string

resource logGroup 'AWS.Logs/LogGroup@default' = {
  alias: logGroupName
  properties: {
    LogGroupName: logGroupName
    RetentionInDays: 14
    Tags: [
      {
        Key: 'testKey'
        Value: 'testValue2'
      }
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}