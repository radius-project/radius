# API models

## Generate Models

```
autorest README.md --tag=v<api-version>
```

---

## Configuration

The following are the settings for this using this API with AutoRest.

### OpenAPI Spec

### Input specificiations

#### Tag: v20220315 specification

```yaml $(tag) == 'v20220315'
input-file:
  - ../../../swagger/specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/environments.json
```

### Common

```yaml
version: 3.*.*
use: "@autorest/go@4.0.0-preview.29"
module-version: 0.0.1
file-prefix: zz_generated_
license-header: MICROSOFT_MIT_NO_VERSION
```

### Output

#### Tag: v20220315 models

These settings apply only when `--tag=v20220315` is specified on the command line.

```yaml $(tag) == 'v20220315'
output-folder: ./v20220315
```
