extension aws

@description('ARN of a public AMI.')
param imageArn string

// Without existing, this declares an AMI to create. Bicep only warns (BCP245), so UCP must reject the deploy.
resource image 'AWS.EC2/Image@default' = {
  alias: 'image'
  properties: {
    Arn: imageArn
  }
}
