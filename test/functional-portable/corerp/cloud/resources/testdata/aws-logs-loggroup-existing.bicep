extension aws

param logGroupName string

resource existingLogGroup 'AWS.Logs/LogGroup@default' existing = {
  alias: logGroupName
  properties: {
    LogGroupName: logGroupName
  }
}

output var string = existingLogGroup.properties.LogGroupName
