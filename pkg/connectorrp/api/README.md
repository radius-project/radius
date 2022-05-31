# API models

This directory includes API version specific models from open api specs. The models in this directory is used for serializing/deserializing request and response. [datamodels](../datamodel/) has the converters to convert between version specific models and datamodels. datamodels will be used for internal controller and datastorage.

## Generate new models
### Prerequisites
1. Install [NodeJS](https://nodejs.org/)
2. Install [AutoRest](http://aka.ms/autorest)
```
npm install -g autorest
```

### Add new api-version

1. Add api version tags and openapi file below in this README.md
2. Run autorest.
```bash
autorest README.md --tag=connector-2022-03-15-privatepreview
```
3. Create or modify the corresponding datamodels in [datamodel](../datamodel/)
4. Add the converter between versioned model and datamodel in [converter](../datamodel/converter/)

---

## Configuration

The following are the settings for this using this API with AutoRest.

### Input OpenAPI specificiations

#### Tag: connector-2022-03-15-privatepreview

These settings apply only when `--tag=connector-2022-03-15-privatepreview` is specified on the command line.

```yaml $(tag) == 'connector-2022-03-15-privatepreview'
input-file:
  - ../../../swagger/specification/applications/resource-manager/Applications.Connector/preview/2022-03-15-privatepreview/mongoDatabases.json
  - ../../../swagger/specification/applications/resource-manager/Applications.Connector/preview/2022-03-15-privatepreview/rabbitMQMessageQueues.json
  - ../../../swagger/specification/applications/resource-manager/Applications.Connector/preview/2022-03-15-privatepreview/daprSecretStores.json
  - ../../../swagger/specification/applications/resource-manager/Applications.Connector/preview/2022-03-15-privatepreview/sqlDatabases.json
  - ../../../swagger/specification/applications/resource-manager/Applications.Connector/preview/2022-03-15-privatepreview/redisCaches.json
  - ../../../swagger/specification/applications/resource-manager/Applications.Connector/preview/2022-03-15-privatepreview/daprInvokeHttpRoutes.json
  - ../../../swagger/specification/applications/resource-manager/Applications.Connector/preview/2022-03-15-privatepreview/daprStateStores.json
```

### Common

The following configuration generates track2 go models and client.

```yaml $(tag) != ''
version: 3.*.*
use: "@autorest/go@4.0.0-preview.29"
module-version: 0.0.1
file-prefix: zz_generated_
license-header: MICROSOFT_MIT_NO_VERSION
```

### Output

#### Tag: connector-2022-03-15-privatepreview

These settings apply only when `--tag=connector-2022-03-15-privatepreview` is specified on the command line.

```yaml $(tag) == 'connector-2022-03-15-privatepreview'
output-folder: ./v20220315privatepreview
```
