### Prerequisites
1. Install [NodeJS](https://nodejs.org/)
2. Install [AutoRest](http://aka.ms/autorest)
```
npm install -g autorest
```
---

## Configuration

The following are the settings for this using this API with AutoRest.

### Input OpenAPI specificiations

#### Tag: 2023-04-15-preview

These settings apply only when `--tag=2023-04-15-preview` is specified on the command line.

```yaml $(tag) == '2023-04-15-preview'
input-file:
  - ../../../pkg/cli/swagger/genericResource.json
modelerfour: 
  treat-type-object-as-anything: false
```

### Common

The following configuration generates track2 go models and client.

```yaml $(tag) != ''
version: 3.*.*
use: "@autorest/go@4.0.0-preview.44"
module-version: 0.0.1
file-prefix: zz_generated_
license-header: MICROSOFT_MIT_NO_VERSION
azure-arm: true
```

### Output

#### Tag: 2023-04-15-preview

These settings apply only when `--tag=2023-04-15-preview` is specified on the command line.

```yaml $(tag) == '2023-04-15-preview'
output-folder: ./generated
```

### Adding ResourceTypes:
All resource types are tracked in resourceTypesList in ucp package. Whenever a new core-rp or link type is added this list has to be updated.
