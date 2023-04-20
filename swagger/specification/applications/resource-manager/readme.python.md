## Python

These settings apply only when `--python` is specified on the command line.
Please also specify `--python-sdks-folder=<path to the root directory of your azure-sdk-for-python clone>`.

``` yaml $(track2)
azure-arm: true
license-header: MICROSOFT_MIT_NO_VERSION
package-name: azure-mgmt-applications
no-namespace-folders: true
package-version: 1.0.0b1
clear-output-folder: true
```

``` yaml $(python-mode) == 'update' && $(track2)
no-namespace-folders: true
output-folder: $(python-sdks-folder)/applications/azure-mgmt-applications/azure/mgmt/applications
```

``` yaml $(python-mode) == 'create' && $(track2)
basic-setup-py: true
output-folder: $(python-sdks-folder)/applications/azure-mgmt-applications
```

### Tag: package-core-2023-04-15-preview and python

These settings apply only when `--tag=package-core-2023-04-15-preview --python` is specified on the command line.
Please also specify `--python-sdks-folder=<path to the root directory of your azure-sdk-for-python clone>`.

``` yaml $(tag) == 'package-core-2023-04-15-preview'
namespace: azure.mgmt.applications.core.v2023_04_15_preview
output-folder: $(python-sdks-folder)/applications/azure-mgmt-applications/azure/mgmt/applications/core/v2023_04_15_preview
python:
  namespace: azure.mgmt.applications.core.v2023_04_15_preview
  output-folder: $(python-sdks-folder)/applications/azure-mgmt-applications/azure/mgmt/applications/core/v2023_04_15_preview
```

### Tag: package-link-2023-04-15-preview and python

These settings apply only when `--tag=package-link-2023-04-15-preview --python` is specified on the command line.
Please also specify `--python-sdks-folder=<path to the root directory of your azure-sdk-for-python clone>`.

``` yaml $(tag) == 'package-link-2023-04-15-preview'
namespace: azure.mgmt.applications.link.v2023_04_15_preview
output-folder: $(python-sdks-folder)/applications/azure-mgmt-applications/azure/mgmt/applications/link/v2023_04_15_preview
python:
  namespace: azure.mgmt.applications.link.v2023_04_15_preview
  output-folder: $(python-sdks-folder)/applications/azure-mgmt-applications/azure/mgmt/applications/link/v2023_04_15_preview
```

### Python multi-api

Generate all API versions currently shipped for this package

```yaml $(multiapi) && $(track2)
clear-output-folder: true
batch:
  - tag: package-core-2023-04-15-preview
  - tag: package-link-2023-04-15-preview
  - multiapiscript: true
```

``` yaml $(multiapiscript)
output-folder: $(python-sdks-folder)/applications/azure-mgmt-applications/azure/mgmt/applications/
clear-output-folder: false
perform-load: false
```
