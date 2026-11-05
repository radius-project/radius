# Native AWS type schemas

Each file here is the schema of an AWS type that has a CloudFormation type, but that AWS Cloud Control can't create in at least one region. UCP serves the type with a native handler in those regions, and with Cloud Control elsewhere. This schema replaces the one CloudFormation returns with either handler, so both handlers store the same identifier, use the same property names, and treat the same properties as write-only.

Use CloudFormation's resource schema format. Name the file after the type, in lowercase with `::` replaced by `-`, such as `aws-docdb-dbcluster.json`. See `pkg/aws/schemas/schemas.go`.

A type can also have a metadata file next to its schema, such as `aws-docdb-dbcluster.metadata.json`, for what CloudFormation's schema format has no field for: `identityScope`, `onDelete`, and `arnProperty`. See `Metadata` in `pkg/aws/schemas/schemas.go` and the "Native handlers" section of `docs/ucp/aws.md`.
