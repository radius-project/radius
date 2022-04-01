# Azure API
> see https://aka.ms/autorest

## Getting Started
To build the SDKs for AIB, simply install AutoRest via `npm` (`npm install -g autorest`) and then run:
> `./generate.sh`

---

## Configuration
The following are the settings for this using this API with AutoRest.

```yaml
# specify the version of Autorest to use
version: 2.*.*
use: "@microsoft.azure/autorest.go@2.1.137"
input-file: ../../../../swagger/specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/environments.json
output-folder: .  # this directory
go:
  namespace: v20220315
```

---
## Note
