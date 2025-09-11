extension aws

param logGroupName string
param retentionInDays string
param creationTimestamp string

resource logGroup 'AWS.Logs/LogGroup@default' = {
  alias: logGroupName
  properties: {
    LogGroupName: logGroupName
    RetentionInDays: int(retentionInDays)
    Tags: [
      {
        Key: 'RadiusCreationTimestamp'
        Value: creationTimestamp
      }
    ]
  }
}
