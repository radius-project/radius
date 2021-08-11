# Radius Open API documents

This directory holds the Open API documents used to generate `pkg/radclient`.

- `common-types.json`: these are shared type definitions for common constructs in ARM. Taken from [here](https://github.com/Azure/azure-rest-api-specs-pr).
- `radius.json`: Open API document for our Radius Resource Provider.
- `traits.json`: Open API document for our ComponentTrait type.

Both of the `common-types.json` and `radius.json` are symlink. The
main reason is that files are also used in our schema validation logic
in the package `pkg/radrp/schema` through go-embed. Due to the
limitation of go-embed, we can not refer to these files directly from
the package `pkg/radrp/schema`, nor can we embed symlinks to these
files. As a workaround, we leave the hard copy of both these JSON
files in `pkg/radrp/schema` and create the symlinks in this directory.

However, the `traits.json` file differs from that of
`pkg/radrp/schema` because autorest does not yet support `oneOf`
polymorphic declaration. In order to still make use of polymorphic
type validations, we use polymorphic declarations in
`pkg/radrp/schema`'s version of `traits.json`. This version only
contains a union of all the traits we know.

> 💡 `radius.json` may reference types defined in the `../application-model` directory for reuse of definitions. You should update the generated code (run `make generate`) when making **any** schema change.

## Authoring

You can find the ARM team's documentation for authoring Open API documents [here](https://github.com/Azure/azure-rest-api-specs/blob/master/documentation/Getting%20started%20with%20OpenAPI%20specifications.md)

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
