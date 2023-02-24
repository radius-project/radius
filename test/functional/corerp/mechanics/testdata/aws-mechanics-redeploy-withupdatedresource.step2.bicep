import aws as aws

param streamName string

resource stream 'AWS.Kinesis/Stream@default' = {
  alias: streamName
  properties: {
    Name: streamName
    RetentionPeriodHours: 48
    ShardCount: 3
  }
}
