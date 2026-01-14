# Terraform Installer API (Radius)

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/installer/terraform/install` | Install a Terraform version |
| `POST` | `/installer/terraform/uninstall` | Uninstall a Terraform version |
| `GET` | `/installer/terraform/status` | Get installer status |

## Install Request

Provide **either** `version` or `sourceUrl` (or both):

```json
{
  "version": "1.6.4",
  "sourceUrl": "https://example.com/terraform.zip",
  "checksum": "sha256:abc123..."
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `version` | One of version/sourceUrl | Semver version (e.g., `1.6.4`, `1.6.4-beta.1`) |
| `sourceUrl` | One of version/sourceUrl | Direct download URL for Terraform archive |
| `checksum` | Recommended | SHA256 checksum (`sha256:<hex>` or bare hex) |

**Notes:**

- If only `sourceUrl` is provided (no version), a version identifier is auto-generated from the URL hash (e.g., `custom-a1b2c3d4`)
- Bare hex checksums are also accepted (without `sha256:` prefix)
- Idempotent: re-installing an existing version promotes it to current without re-downloading

## Uninstall Request

```json
{
  "version": "1.6.4"
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `version` | No | Version to uninstall (defaults to current version if omitted) |

**Notes:**

- Uninstalling the current version switches to the previous version (if available) or clears current
- Blocked if Terraform executions are in progress (when `ExecutionChecker` is configured)

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
  "lastError": "",
  "lastUpdated": "2025-01-06T10:30:00Z"
}
```

| State | Description |
|-------|-------------|
| `not-installed` | No Terraform version installed |
| `installing` | Installation in progress |
| `ready` | Terraform installed and ready |
| `uninstalling` | Uninstallation in progress |
| `failed` | Last operation failed (see `lastError`) |

## Configuration

| Config Key | Description | Default |
|------------|-------------|---------|
| `terraform.path` | Root directory for Terraform installations | `/terraform` |
| `terraform.sourceBaseUrl` | Mirror/base URL for downloads (air-gapped setups) | `https://releases.hashicorp.com` |

## Behavior

- **Concurrency:** Only one install/uninstall runs at a time; concurrent requests receive `installer is busy`
- **Archive Detection:** Supports both ZIP archives and plain binaries (detected via magic bytes)
- **Cleanup:** Downloaded archives are automatically removed after extraction
- **Symlink:** Current version is symlinked at `{terraform.path}/current`
