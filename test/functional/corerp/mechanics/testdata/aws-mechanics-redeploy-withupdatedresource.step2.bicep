import aws as aws

resource stream 'AWS.Kinesis/Stream@default' = {
  name: 'my-stream'
  properties: {
    Name: 'my-stream'
    RetentionPeriodHours: 48
    ShardCount: 3
  }
}
