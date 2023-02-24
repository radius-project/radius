import aws as aws

param filterName string
param logGroupName string

resource metricsFilter 'AWS.Logs/MetricFilter@default' = {
  alias: filterName
  properties: {
    FilterName: filterName
    LogGroupName: logGroup.properties.LogGroupName
    FilterPattern: '[ip, identity, user_id, timestamp, request, status_code = 404, size]'
    MetricTransformations: [
      {
        MetricName: '404Count'
        MetricNamespace: 'WebServer/404s'
        MetricValue: '1'
      }
    ]
  }
}

resource logGroup 'AWS.Logs/LogGroup@default' = {
  alias: logGroupName
  properties:{
    LogGroupName:logGroupName
  }
}
