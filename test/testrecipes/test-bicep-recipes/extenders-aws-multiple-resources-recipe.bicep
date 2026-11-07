extension aws

@description('Name and alias of the log group.')
param logGroupName string

@description('Name and alias of the query definition. The test finds it in AWS by this name.')
param queryDefinitionName string

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

// AWS assigns the identifier of a query definition, so UCP deletes it by the identifier it recorded for the alias.
resource queryDefinition 'AWS.Logs/QueryDefinition@default' = {
  alias: queryDefinitionName
  properties: {
    Name: queryDefinitionName
    QueryString: 'fields @timestamp, @message | limit 1'
  }
}
