extension aws

@description('Name and alias of the query definition. The test finds it in AWS by this name.')
param queryDefinitionName string

// AWS assigns the identifier of a query definition, so UCP finds it by its alias.
resource queryDefinition 'AWS.Logs/QueryDefinition@default' = {
  alias: queryDefinitionName
  properties: {
    Name: queryDefinitionName
    QueryString: 'fields @timestamp, @message | limit 1'
  }
}
