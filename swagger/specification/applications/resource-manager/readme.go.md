## Go

These settings apply only when `--go` is specified on the command line.

```yaml $(go) && !$(track2)
go:
  license-header: MICROSOFT_MIT_NO_VERSION
  namespace: applications
  clear-output-folder: true
```

### Go multi-api

```yaml $(go) && $(multiapi)
batch:
  - tag: package-core-2023-04-15-preview
  - tag: package-link-2023-04-15-preview
```

### Tag: package-core-2023-04-15-preview and go

These settings apply only when `--tag=package-core-2023-04-15-preview --go` is specified on the command line.
Please also specify `--go-sdk-folder=<path to the root directory of your azure-sdk-for-go clone>`.

```yaml $(tag) == 'package-core-2023-04-15-preview' && $(go)
output-folder: $(go-sdk-folder)/services/preview/$(namespace)/mgmt/2023-04-15-preview/core
```

### Tag: package-link-2023-04-15-preview and go

These settings apply only when `--tag=package-link-2023-04-15-preview --go` is specified on the command line.
Please also specify `--go-sdk-folder=<path to the root directory of your azure-sdk-for-go clone>`.

```yaml $(tag) == 'package-link-2023-04-15-preview' && $(go)
output-folder: $(go-sdk-folder)/services/preview/$(namespace)/mgmt/2023-04-15-preview/link
```
