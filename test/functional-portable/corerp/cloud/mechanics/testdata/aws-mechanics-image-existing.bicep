extension aws
extension radius

@description('Environment for the extender that records the image values.')
param environment string

@description('Name of the extender that records the image values.')
param extenderName string

@description('ARN of a public AMI. Arn is the primary identifier of AWS::EC2::Image.')
param imageArn string

// AWS::EC2::Image is read-only: AWS creates it, and a template can only reference it with existing.
resource image 'AWS.EC2/Image@default' existing = {
  alias: 'image'
  properties: {
    Arn: imageArn
  }
}

// The extender keeps values UCP read from AWS, so the test can check them.
resource extender 'Applications.Core/extenders@2023-10-01-preview' = {
  name: extenderName
  properties: {
    environment: environment
    resourceProvisioning: 'manual'
    imageId: image.properties.ImageId
    imageState: image.properties.State
  }
}
