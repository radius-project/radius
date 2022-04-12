# Radius

> see https://aka.ms/autorest


---

## Getting Started

To build the SDK for Radius, simply [Install AutoRest](https://aka.ms/autorest/install) and in this folder, run:

> `autorest`

To see additional help and options, run:

> `autorest --help`

---

## Configuration

### Basic Information

These are the global settings for the Radius API.

``` yaml
title: Radius
description: Radius
openapi-type: arm
```

### Tag: radius-2022-04-11-preview

These settings apply only when `--tag=radius-2022-04-11-preview` is specified on the command line.

```yaml $(tag) == 'radius-2021-01-01-preview'
input-file:
  - Microsoft.Radius/preview/2022-04-11-preview/radius.json
```