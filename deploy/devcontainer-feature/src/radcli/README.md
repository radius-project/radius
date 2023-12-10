# Radius CLI

Installs the [Radius CLI](https://github.com/radius-project/radius) along with needed dependencies.

## Example Usage - Install latest stable release

This will install the latest stable release of the `rad` CLI.

```json
"features": {
    "ghcr.io/devcontainers/radius/radiuscli:latest": {
        "version": "latest"
    }
}
```

## Example Usage - Install edge release

This will install the edge (unstable) release of the `rad` CLI.

```json
"features": {
    "ghcr.io/devcontainers/radius/radiuscli:latest": {
        "version": "edge"
    }
}
```

## Example Usage - Install a specific version

This will install version 0.28.0 of the `rad` CLI.

```json
"features": {
    "ghcr.io/devcontainers/radius/radiuscli:latest": {
        "version": "0.28.0"
    }
}
```

## Options

| Options Id | Description | Type | Default Value |
|-----|-----|-----|-----|
| version |  Select or enter a `rad` CLI version. Available versions are "latest" (stable), "edge" (unstable), or any specific version like `0.28.0`. | string | latest |
