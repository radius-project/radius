import aws as aws

param streamName string

resource stream 'AWS.Kinesis/Stream@default' = {
  name: streamName
  properties: {
    Name: streamName
    RetentionPeriodHours: 168
    ShardCount: 3
  }
}
