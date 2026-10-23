# Helm Chart Tests

This directory contains unit tests for the Radius Helm chart templates.

## Prerequisites

Install the helm-unittest plugin:

```bash
helm plugin install https://github.com/helm-unittest/helm-unittest.git --version 1.1.1 --verify=false
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

### Encryption Secret upgrades

`encryption_secret_test.yaml` uses the plugin's Kubernetes provider to exercise real template `lookup` calls against existing Secret fixtures. It covers preserved key-store bytes and rotation metadata, legacy missing annotations, malformed metadata, and refusal to replace missing or empty key data. Nonempty key data is treated as opaque: an upgrade never attempts to repair or regenerate a malformed store.

For server-side apply, run the focused Go regression from the repository root on Linux or macOS with Go, Helm 4, and the repository's envtest binaries (envtest is not supported on Windows):

```bash
KUBEBUILDER_ASSETS="$(go tool setup-envtest use -p path '1.30.*')" \
  go test ./pkg/cli/helm -run '^TestEncryptionSecretUpgrade$' -count=1
```

The test starts its own API server and etcd, uses a temporary kubeconfig, and installs the shipping encryption Secret template without control-plane workloads. It exercises the Radius Helm client with the staged `rad_linux_amd64` manager followed by the standalone Helm binary, as well as ordinary `rad` upgrades. Fresh, previously rotated, and legacy Secrets must keep their UID, key-data SHA-256 digest, and rotation annotation presence/value. It never logs key data or uses an existing cluster. `make test` also runs this regression; direct `go test` skips it when `KUBEBUILDER_ASSETS` is unset. This focused regression does not replace full installation or previous-supported-release upgrade verification.

## Test Structure

The tests validate:

- The `radius.image` helper template correctly constructs image URLs
- Default registry (ghcr.io/radius-project) is used when `global.imageRegistry` is not set
- Custom registry is properly prepended when `global.imageRegistry` is set
- `global.imageTag` is used when component tag is not specified
- Tag priority is respected: component tag > global.imageTag > appVersion
- `global.imagePullSecrets` are correctly applied to pod specs when specified
- Multiple image pull secrets are properly handled
- All deployments and statefulsets use the helper correctly

## Adding New Tests

When adding new templates that reference container images, ensure they:

1. Use the `radius.image` helper for image construction:

   ```yaml
   image: "{{ include "radius.image" (dict "image" .Values.component.image "tag" (.Values.component.tag | default .Values.global.imageTag | default $appversion) "global" .Values.global) }}"
   ```

2. Include imagePullSecrets support in the pod spec:

   ```yaml
   {{- if .Values.global.imagePullSecrets }}
   imagePullSecrets:
     {{- toYaml .Values.global.imagePullSecrets | nindent 2 }}
   {{- end }}
   ```

The tag priority system ensures:

1. Component-specific tag is used if specified
2. Falls back to `global.imageTag` if component tag is not set
3. Finally falls back to chart's appVersion if neither is set

Then add corresponding tests to validate:

- Image construction with various registry and tag combinations
- imagePullSecrets are correctly applied when specified
- imagePullSecrets are omitted when not specified
