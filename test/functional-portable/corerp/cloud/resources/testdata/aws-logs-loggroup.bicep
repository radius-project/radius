extension aws

param creationTimestamp string
param logGroupName string

resource logGroup 'AWS.Logs/LogGroup@default' = {
  alias: logGroupName
  properties: {
    LogGroupName: logGroupName
    RetentionInDays: 7
    Tags: [
      {
        Key: 'testKey'
        Value: 'testValue'
      }
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}