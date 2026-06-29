# API models

This directory includes API version specific models from open api specs. The models in this directory is used for serializing/deserializing request and response. [datamodels](../datamodel/) has the converters to convert between version specific models and datamodels. datamodels will be used for internal controller and datastorage.

## Generate new models

The versioned models in this directory are generated from the TypeSpec definitions in the
[`typespec`](../../../typespec/) directory using the [TypeSpec Go emitter](https://github.com/Azure/typespec-azure)
(`@azure-tools/typespec-go`). AutoRest is no longer used to generate these models (see
[radius-project/radius#11425](https://github.com/radius-project/radius/issues/11425)).

### Prerequisites

1. Install [NodeJS](https://nodejs.org/)

### Add new api-version

1. Create or update the applicable TypeSpec files under the matching project in the [`typespec`](../../../typespec/) directory.
2. Generate the models and client by running:

    ```bash
    make generate-rad-daprrp-client
    ```

    (or `make generate` to regenerate every namespace).
3. Create or modify the corresponding datamodels in [datamodel](../datamodel/)
4. Add the converter between versioned model and datamodel in [converter](../datamodel/converter/)
