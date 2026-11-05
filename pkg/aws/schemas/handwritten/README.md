# Hand-written AWS type schemas

Each file here is the schema of an AWS resource that has no CloudFormation type, such as an S3 object. UCP always serves these types with a native handler and never asks CloudFormation about them.

Use CloudFormation's resource schema format, with the AWS API's own parameter names in PascalCase. Name the file after the type, in lowercase with `::` replaced by `-`, such as `aws-s3-object.json`. See `pkg/aws/schemas/schemas.go`.
