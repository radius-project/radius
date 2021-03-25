# Radius Open API documents

This directory holds the Open API documents used to generate `pkg/radclient`. 

- `common-types.json`: these are shared type definitions for common constructs in ARM. Taken from [here](https://github.com/Azure/azure-rest-api-specs-pr).
- `radius.json`: Open API document for our Radius Resource Provider.

> 💡 `radius.json` may reference types defined in the `../application-model` directory for reuse of definitions. You should update the generated code (run `make generate`) when making **any** schema change.

## Configuration

This section is configuration for autorest. Do not modify these code blocks without consulting the autorest documentation first. Modifications here will change the behavior of the code generator.

### Basic Information

These are the global settings for the radius.

```yaml
openapi-type: arm
tag: package-2018-09-01-preview
```

### Tag: package-2018-09-01-preview

These settings apply only when `--tag=package-2018-09-01-preview` is specified on the command line.

```yaml $(tag) == 'package-2018-09-01-preview'
input-file:
  - radius.json
```

### Go

These settings apply only when `--go` is specified on the command line.

```yaml $(go)
go:
  license-header: MICROSOFT_MIT_NO_VERSION
  namespace: radius
  clear-output-folder: false
```

