## Schemas in Radius

This directory is a catalog of our JSON schemas and Open API documents 

We use these for a variety of purposes:

- Code generation of `pkg/radclient` (Azure SDK standin)
- (Future) Validation of incoming RP requests
- (Future) Bicep language support
- (Future) Generating documenation for the Radius application model

### The catalog

- `rest-api-specs` Open API documents that represent the contract of our Resource Provider.
- `application-model` JSON schemas that represent the types in the Radius application model.

### Making Changes

When making changes to any of these schemas, you need to run:

```sh
make generate
```

This will update all of the generated code for the whole project. Changes to generated should be committed along with the changes to schemas.
