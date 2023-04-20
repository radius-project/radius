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
  - package-link: true
```

### Batch settings: multi-api
These settings are for batch mode only: (ie, add `--multiapi` to the command line )

``` yaml $(multiapi)
namespace: Applications.$(ApiVersionName)
output-folder: $(csharp-sdks-folder)/$(ApiVersionName)/Generated

batch:
  - core-2023-04-15-preview: true
    ApiVersionName: Api2022_03_15_privatepreview
  - link-2023-04-15-preview: true
    ApiVersionName: Api2022_03_15_privatepreview
```

``` yaml $(core-2023-04-15-preview)
tag: package-core-2023-04-15-preview
```

``` yaml $(link-2023-04-15-preview)
tag: package-link-2023-04-15-preview
```

### Tag: package-core-2023-04-15-preview
``` yaml $(tag) == 'package-core-2023-04-15-preview'
output-folder: $(csharp-sdks-folder)/applications/management/2023-04-15-preview/core/src/Generated
input-file:
- Applications.Core/preview/2023-04-15-preview/global.json
- Applications.Core/preview/2023-04-15-preview/environments.json
- Applications.Core/preview/2023-04-15-preview/applications.json
```

### Tag: package-link-2023-04-15-preview
``` yaml $(tag) == 'package-link-2023-04-15-preview'
output-folder: $(csharp-sdks-folder)/applications/management/2023-04-15-preview/link/src/Generated
input-file:
- Applications.Link/preview/2023-04-15-preview/openapi.json
- Applications.Link/preview/2023-04-15-preview/extenders.json
```