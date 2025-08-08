# Helm Chart Tests

This directory contains unit tests for the Radius Helm chart templates.

## Prerequisites

Install the helm-unittest plugin:

```bash
helm plugin install https://github.com/helm-unittest/helm-unittest.git
```

## Running Tests

From the Chart directory:

```bash
helm unittest .
```

To run with verbose output:

```bash
helm unittest . -v
```

To run a specific test file:

```bash
helm unittest . -f tests/helpers_test.yaml
```

## Test Structure

The tests validate:

- The `radius.image` helper template correctly constructs image URLs
- Default registry (ghcr.io/radius-project) is used when `global.imageRegistry` is not set
- Custom registry is properly prepended when `global.imageRegistry` is set
- `global.imageTag` is used when component tag is not specified
- Tag priority is respected: component tag > global.imageTag > appVersion
- All deployments and statefulsets use the helper correctly

## Adding New Tests

When adding new templates that reference container images, ensure they use the `radius.image` helper:

```yaml
image: "{{ include "radius.image" (dict "image" .Values.component.image "tag" (.Values.component.tag | default .Values.global.imageTag | default $appversion) "global" .Values.global) }}"
```

The tag priority system ensures:

1. Component-specific tag is used if specified
2. Falls back to `global.imageTag` if component tag is not set
3. Finally falls back to chart's appVersion if neither is set

Then add corresponding tests to validate the image construction.
