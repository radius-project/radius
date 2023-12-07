# Radius CLI

Installs the [Radius CLI](https://github.com/radius-project/radius) along with needed dependencies.

## Example Usage - Install latest release

```json
"features": {
    "ghcr.io/devcontainers/radius/radiuscli:latest": {
        "version": "latest"
    }
}
```

## Example Usage - Install edge release

```json
"features": {
    "ghcr.io/devcontainers/radius/radiuscli:latest": {
        "version": "edge"
    }
}
```

## Options

| Options Id | Description | Type | Default Value |
|-----|-----|-----|-----|
| version | Select or enter an Radius CLI version. Available versions are "latest" and "edge" | string | latest |
