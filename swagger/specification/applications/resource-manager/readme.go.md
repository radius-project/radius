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
  - tag: package-core-2022-03-15-privatepreview
  - tag: package-connector-2022-03-15-privatepreview
```

### Tag: package-core-2022-03-15-privatepreview and go

These settings apply only when `--tag=package-core-2022-03-15-privatepreview --go` is specified on the command line.
Please also specify `--go-sdk-folder=<path to the root directory of your azure-sdk-for-go clone>`.

```yaml $(tag) == 'package-core-2022-03-15-privatepreview' && $(go)
output-folder: $(go-sdk-folder)/services/preview/$(namespace)/mgmt/2022-03-15-privatepreview/core
```

### Tag: package-connector-2022-03-15-privatepreview and go

These settings apply only when `--tag=package-connector-2022-03-15-privatepreview --go` is specified on the command line.
Please also specify `--go-sdk-folder=<path to the root directory of your azure-sdk-for-go clone>`.

```yaml $(tag) == 'package-connector-2022-03-15-privatepreview' && $(go)
output-folder: $(go-sdk-folder)/services/preview/$(namespace)/mgmt/2022-03-15-privatepreview/connector
```
