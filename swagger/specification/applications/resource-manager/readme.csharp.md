## C#

These settings apply only when `--csharp` is specified on the command line.
Please also specify `--csharp-sdks-folder=<path to "SDKs" directory of your azure-sdk-for-net clone>`.

```yaml $(csharp)
csharp:
  azure-arm: true
  license-header: MICROSOFT_MIT_NO_VERSION
  payload-flattening-threshold: 1
  clear-output-folder: true
  client-side-validation: false
```

``` yaml $(csharp) && !$(multiapi) && !$(csharp-profile)
namespace: Applications
output-folder: $(csharp-sdks-folder)/applications/management/src/Generated

batch:
  - package-core: true
  - package-connector: true
```

### Batch settings: multi-api
These settings are for batch mode only: (ie, add `--multiapi` to the command line )

``` yaml $(multiapi)
namespace: Applications.$(ApiVersionName)
output-folder: $(csharp-sdks-folder)/$(ApiVersionName)/Generated

batch:
  - core-2022-03-15-privatepreview: true
    ApiVersionName: Api2022_03_15_privatepreview
  - connector-2022-03-15-privatepreview: true
    ApiVersionName: Api2022_03_15_privatepreview
```

``` yaml $(core-2022-03-15-privatepreview)
tag: package-core-2022-03-15-privatepreview
```

``` yaml $(connector-2022-03-15-privatepreview)
tag: package-connector-2022-03-15-privatepreview
```

### Tag: package-core-2022-03-15-privatepreview
``` yaml $(tag) == 'package-core-2022-03-15-privatepreview'
output-folder: $(csharp-sdks-folder)/applications/management/2022-03-15-privatepreview/core/src/Generated
input-file:
- Applications.Core/preview/2022-03-15-privatepreview/global.json
- Applications.Core/preview/2022-03-15-privatepreview/environments.json
- Applications.Core/preview/2022-03-15-privatepreview/applications.json
```

### Tag: package-connector-2022-03-15-privatepreview
``` yaml $(tag) == 'package-connector-2022-03-15-privatepreview'
output-folder: $(csharp-sdks-folder)/applications/management/2022-03-15-privatepreview/connector/src/Generated
input-file:
- Applications.Connector/preview/2022-03-15-privatepreview/global.json
- Applications.Connector/preview/2022-03-15-privatepreview/mongoDatabases.json
```