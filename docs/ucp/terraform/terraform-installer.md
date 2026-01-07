# Terraform Installer API (Radius)

## Information

- Endpoints (UCP): `POST /installer/terraform/install`, `POST /installer/terraform/uninstall`, `GET /installer/terraform/status`.
- Provide either `version` (semver-like) or `sourceUrl`; a checksum (`sha256:<hex>`) is strongly recommended.
- Default downloads use `https://releases.hashicorp.com`; configure a mirror via `terraform.sourceBaseUrl` in config.
- Only one install/uninstall runs at a time; concurrent requests receive `installer is busy`.
