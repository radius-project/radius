import aws as aws

resource stream 'AWS.Kinesis/Stream@default' = {
  name: 'streamy'
  properties: {
    RetentionPeriodHours: 168
    ShardCount: 3
  }
}
