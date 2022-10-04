import aws as aws

resource stream 'AWS.Kinesis/Stream@default' = {
  name: 'my-stream'
  properties: {
    Name: 'my-stream'
    RetentionPeriodHours: 168
    ShardCount: 3
  }
}
