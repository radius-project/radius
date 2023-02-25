import aws as aws

param streamName string

resource stream 'AWS.Kinesis/Stream@default' = {
  alias: streamName
  properties: {
    Name: streamName
    RetentionPeriodHours: 168
    ShardCount: 3
  }
}
