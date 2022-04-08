# API models

## Generate models

```
autorest README.md --tag=v<api-version>
```

---

## Configuration

The following are the settings for this using this API with AutoRest.

### Input OpenAPI specificiations

#### Tag: 2022-03-15-privatepreview

These settings apply only when `--tag=2022-03-15-privatepreview` is specified on the command line.

```yaml $(tag) == '2022-03-15-privatepreview'
input-file:
  - ../../../swagger/specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/environments.json
```

### Common

```yaml $(tag) != ''
version: 3.*.*
use: "@autorest/go@4.0.0-preview.29"
module-version: 0.0.1
file-prefix: zz_generated_
license-header: MICROSOFT_MIT_NO_VERSION
```

### Output

#### Tag: 2022-03-15-privatepreview

These settings apply only when `--tag=2022-03-15-privatepreview` is specified on the command line.

```yaml $(tag) == '2022-03-15-privatepreview'
output-folder: ./v20220315privatepreview
```
