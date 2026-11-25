extension aws

@description('Name and alias of the log group.')
param logGroupName string

@description('Unix time the test started, used by the job that purges leaked test resources.')
param creationTimestamp string

// The same log group as aws-mechanics-redeploy-withupdatedresource.bicep, without RetentionInDays.
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
