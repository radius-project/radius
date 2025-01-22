# basic

## Configuration

### Information

```yaml
title: Basic
description: Contains a set of basic spec samples for integration tests
openapi-type: arm
tag: package-2021-10-31
```

### Tag: package-2021-10-31

These settings apply only when `--tag=package-2021-10-31` is specified on the command line.

```yaml $(tag) == 'package-2021-10-31'
input-file:
  - Test.Rp1/stable/2021-10-31/spec.json
```