# Hand-written AWS type schemas

Each file here is the schema of an AWS resource that has no CloudFormation type, such as an S3 object. UCP always serves these types with a native handler and never asks CloudFormation about them.

Use CloudFormation's resource schema format, with the AWS API's own parameter names in PascalCase, and list the AWS permissions of each operation under `handlers`. Name the file after the type, in lowercase with `::` replaced by `-`, such as `aws-s3-object.json`, directly in this directory. UCP fails to load a schema that breaks these rules. See "Hand-written types" in `docs/ucp/aws.md`, and the test-only example in `pkg/ucp/aws/native/nativetest/testdata/handwritten`.

A type can also have a metadata file next to its schema, such as `aws-iam-accountpasswordpolicy.metadata.json`, for what CloudFormation's schema format has no field for: `identityScope`, `onDelete`, and `arnProperty`. See `Metadata` in `pkg/aws/schemas/schemas.go` and the "Native handlers" section of `docs/ucp/aws.md`.
