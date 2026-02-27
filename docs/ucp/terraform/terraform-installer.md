# Terraform Installer API (Radius)

## Endpoints

| Method | Path                             | Description                   |
| ------ | -------------------------------- | ----------------------------- |
| `POST` | `/installer/terraform/install`   | Install a Terraform version   |
| `POST` | `/installer/terraform/uninstall` | Uninstall a Terraform version |
| `GET`  | `/installer/terraform/status`    | Get installer status          |

## Install Request

Provide **either** `version` or `sourceUrl` (or both):

```json
{
  "version": "1.6.4",
  "sourceUrl": "https://example.com/terraform.zip",
  "checksum": "sha256:abc123...",
  "caBundle": "<PEM-encoded CA cert>",
  "authHeader": "Bearer <token>",
  "clientCert": "<PEM-encoded client cert>",
  "clientKey": "<PEM-encoded client key>",
  "proxyUrl": "http://proxy:8080"
}
```

| Field        | Required                 | Description                                                               |
| ------------ | ------------------------ | ------------------------------------------------------------------------- |
| `version`    | One of version/sourceUrl | Semver version (e.g., `1.6.4`, `1.6.4-beta.1`)                            |
| `sourceUrl`  | One of version/sourceUrl | Direct download URL for Terraform archive                                 |
| `checksum`   | Recommended              | SHA256 checksum (`sha256:<hex>` or bare hex)                              |
| `caBundle`   | No                       | PEM-encoded CA cert for self-signed TLS (requires `sourceUrl`)            |
| `authHeader` | No                       | Authorization header for private registries (requires `sourceUrl`)        |
| `clientCert` | No                       | PEM-encoded client cert for mTLS (requires `sourceUrl` and `clientKey`)   |
| `clientKey`  | No                       | PEM-encoded client private key for mTLS (requires `sourceUrl` and `clientCert`) |
| `proxyUrl`   | No                       | HTTP/HTTPS proxy URL (requires `sourceUrl`)                               |

**Notes:**

- If only `sourceUrl` is provided (no version), a version identifier is auto-generated from the URL hash (e.g., `custom-a1b2c3d4`)
- Bare hex checksums are also accepted (without `sha256:` prefix)
- Idempotent: re-installing an existing version promotes it to current without re-downloading

**Private Registry Options:**

- All private registry options (`caBundle`, `authHeader`, `clientCert`, `clientKey`, `proxyUrl`) require `sourceUrl`
- `clientCert` and `clientKey` must be specified together for mTLS
- `proxyUrl` must use `http://` or `https://` scheme

## Uninstall Request

```json
{
  "version": "1.6.4",
  "purge": false
}
```

| Field     | Required | Description                                                        |
| --------- | -------- | ------------------------------------------------------------------ |
| `version` | No       | Version to uninstall (defaults to current version if omitted)      |
| `purge`   | No       | Remove version metadata from database (default: false, keep audit) |

**Notes:**

- Uninstalling the current version switches to the previous version (if available) or clears current
- Blocked if Terraform executions are in progress (when `ExecutionChecker` is configured)
- When `purge: false` (default), version metadata remains with state `Uninstalled` for audit purposes
- When `purge: true`, version metadata is deleted from the database entirely

## Status Response

```json
{
  "currentVersion": "1.6.4",
  "state": "ready",
  "binaryPath": "/terraform/versions/1.6.4/terraform",
  "installedAt": "2025-01-06T10:30:00Z",
  "source": {
    "url": "https://releases.hashicorp.com/terraform/1.6.4/terraform_1.6.4_linux_amd64.zip",
    "checksum": "sha256:abc123..."
  },
  "queue": {
    "pending": 0,
    "inProgress": null
  },
  "versions": { ... },
  "history": [ ... ],
  "lastError": "",
  "lastUpdated": "2025-01-06T10:30:00Z"
}
```

| State           | Description                             |
| --------------- | --------------------------------------- |
| `not-installed` | No Terraform version installed          |
| `installing`    | Installation in progress                |
| `ready`         | Terraform installed and ready           |
| `uninstalling`  | Uninstallation in progress              |
| `failed`        | Last operation failed (see `lastError`) |

## Configuration

| Config Key                | Description                                       | Default                          |
| ------------------------- | ------------------------------------------------- | -------------------------------- |
| `terraform.path`          | Root directory for Terraform installations        | `/terraform`                     |
| `terraform.sourceBaseUrl` | Mirror/base URL for downloads (air-gapped setups) | `https://releases.hashicorp.com` |

## Behavior

- **Concurrency:** Only one install/uninstall runs at a time; concurrent requests receive `installer is busy`
- **Archive Detection:** Supports both ZIP archives and plain binaries (detected via magic bytes)
- **Cleanup:** Downloaded archives are automatically removed after extraction
- **Symlink:** Current version is symlinked at `{terraform.path}/current`
