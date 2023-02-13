import aws as aws

param streamName string

resource stream 'AWS.Kinesis/Stream@default' = {
  name: streamName
  properties: {
    Name: streamName
    RetentionPeriodHours: 24
    ShardCount: 3
  }
}
