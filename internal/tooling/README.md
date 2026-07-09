# External Tool Updater

## Purpose

The external tool updater keeps downloaded CLI versions and SHA-256 checksums in [`build/tools.yaml`](../../build/tools.yaml). [`manifest.go`](manifest.go) validates the manifest, [`updater.go`](updater.go) checks release sources, and [`cmd/tool-updater/main.go`](../../cmd/tool-updater/main.go) exposes the commands used by Make and CI.

The generated [`build/tools.generated.mk`](../../build/tools.generated.mk) file is committed and supplies the metadata to the installer recipes in [`build/tools.mk`](../../build/tools.mk). Do not edit the generated file directly.

## Commands

Run the updater through Make:

```sh
make update-tools
```

Make builds the updater into `bin/` before executing it. This stable path is important on Windows, where security software may block the temporary executable created by `go run`.

To invoke the compiled updater directly:

```sh
bin/tool-updater.exe update --manifest build/tools.yaml --makefile build/tools.generated.mk
```

Use `bin/tool-updater` instead of `bin/tool-updater.exe` on non-Windows systems. The other subcommand regenerates only the Make include:

```sh
bin/tool-updater.exe generate-make --manifest build/tools.yaml --output build/tools.generated.mk
```

## Manifest reference

The manifest is YAML with the following top-level properties:

| Property        | Type            | Allowed or required values                                                                         | Description                                            |
|-----------------|-----------------|----------------------------------------------------------------------------------------------------|--------------------------------------------------------|
| `schemaVersion` | integer         | `1`                                                                                                | Manifest schema version.                               |
| `platforms`     | list of strings | One or more unique values matching `linux_amd64`, `linux_arm64`, `darwin_amd64`, or `darwin_arm64` | Platforms for which checksums and assets are recorded. |
| `tools`         | list of objects | One or more tools                                                                                  | Tool definitions described below.                      |

### Tool properties

| Property           | Type    | Allowed or required values                                                            | Description                                                                                       |
|--------------------|---------|---------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------|
| `name`             | string  | Lowercase letters, numbers, and hyphens; must start with a lowercase letter or number | Stable tool identifier.                                                                           |
| `makePrefix`       | string  | Uppercase letters, numbers, and underscores; must start with an uppercase letter      | Prefix used for generated Make variables such as `YQ_VERSION`.                                    |
| `version`          | string  | Non-empty                                                                             | Currently pinned release version. The value may include a leading `v` when the upstream uses one. |
| `update`           | boolean | `true` or `false`; omitted means `true`                                               | Set to `false` to check the source while keeping the current version pinned.                      |
| `notes`            | string  | Optional                                                                              | Human-readable compatibility or pinning rationale.                                                |
| `source`           | object  | Required                                                                              | Describes how the latest version is discovered.                                                   |
| `downloadTemplate` | string  | Required unless `checksumSource.type` is `none`                                       | URL template for an asset download.                                                               |
| `platforms`        | map     | Required for checksum-bearing tools; must contain every top-level platform            | Asset and checksum data for each supported platform.                                              |
| `checksumSource`   | object  | Required                                                                              | Describes how the updater obtains or computes SHA-256 checksums.                                  |
| `versionFiles`     | list    | Optional                                                                              | Additional repository files whose embedded version must stay synchronized.                        |

### `source` properties

| Property     | Type   | Allowed or required values                                 | Description                                                              |
|--------------|--------|------------------------------------------------------------|--------------------------------------------------------------------------|
| `type`       | string | `github-release`, `stable-text`, or `hashicorp-checkpoint` | Version source parser selected by the updater.                           |
| `repository` | string | Required for `github-release`, in `owner/repository` form  | GitHub repository containing the release.                                |
| `tagPrefix`  | string | Optional                                                   | Prefix added to the pinned version to form a release tag, such as `jq-`. |
| `latestURL`  | string | Non-empty HTTPS URL                                        | Endpoint queried for the latest version.                                 |

For `github-release`, the endpoint must return a GitHub release object with `tag_name`. `stable-text` reads the trimmed response body, and `hashicorp-checkpoint` reads `current_version` from the JSON response.

### `checksumSource` properties

| `type`                | Required properties                                                      | Behavior                                                                                        |
|-----------------------|--------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------|
| `github-release-file` | `fileTemplate`, `format`; also `orderFileTemplate` when `format` is `yq` | Reads a checksum file from the GitHub release.                                                  |
| `url-file`            | `urlTemplate`, `format`                                                  | Reads a checksum file from an arbitrary URL.                                                    |
| `download`            | None                                                                     | Downloads the asset and hashes its bytes locally.                                               |
| `none`                | `integrity`                                                              | Records a non-SHA-256 integrity method, such as `go-sumdb`; the tool has no platform asset map. |

Supported checksum formats are `standard` (filename in the second column), `basename` (compare only the filename after a path), `first` (use the first field), and `yq` (use `checksums_hashes_order` to locate the SHA-256 column). Every stored checksum must be exactly 64 lowercase hexadecimal characters.

### Platform entries

Each platform entry has these properties:

| Property   | Type   | Allowed or required values                | Description                                             |
|------------|--------|-------------------------------------------|---------------------------------------------------------|
| `asset`    | string | Non-empty; may contain template variables | Release asset name.                                     |
| `checksum` | string | Lowercase 64-character SHA-256 value      | Expected checksum for the asset.                        |
| `os`       | string | Optional                                  | Overrides the operating-system value used in templates. |
| `arch`     | string | Optional                                  | Overrides the architecture value used in templates.     |

### Templates

Asset, download, checksum-file, and checksum-URL templates may use these variables:

| Variable         | Value                                                     |
|------------------|-----------------------------------------------------------|
| `{repository}`   | `source.repository`                                       |
| `{tag}`          | `source.tagPrefix` plus the requested version             |
| `{version}`      | Requested version, including its leading `v` when present |
| `{version_no_v}` | Requested version without a leading `v`                   |
| `{asset}`        | Expanded platform asset name                              |
| `{os}`           | Platform OS, or the entry's `os` override                 |
| `{arch}`         | Platform architecture, or the entry's `arch` override     |

Unknown or unterminated template variables are errors.

### Version files

A `versionFiles` entry keeps another repository file synchronized when a tool version changes:

| Property | Type   | Allowed or required values                | Description                                                               |
|----------|--------|-------------------------------------------|---------------------------------------------------------------------------|
| `path`   | string | Repository-relative path                  | File to update. Paths outside the repository are rejected.                |
| `format` | string | `plain` or `replace`                      | Replace the whole file with the version, or replace text between markers. |
| `prefix` | string | Required for `replace`; empty for `plain` | Text immediately before the embedded version.                             |
| `suffix` | string | Required for `replace`; empty for `plain` | Text immediately after the embedded version.                              |

The `replace` prefix must occur exactly once. Terraform uses `versionFiles` for `.terraform-version`, the Go fallback, and the Helm chart default.

## Update behavior

- The updater checks the latest version source for every tool.
- A version changes only when the source is a greater semantic version; downgrades are ignored.
- A tool with `update: false` remains pinned, but its source and current-version checks still run.
- Checksums are refreshed for every configured platform at the selected version.
- The manifest, generated Make include, Terraform compatibility file, and declared version consumers are updated only after source checks succeed.

## Verification

Run the focused tests and static checks after changing the updater or manifest:

```sh
go test ./internal/tooling ./cmd/tool-updater
go vet ./internal/tooling ./cmd/tool-updater
make --no-print-directory -n update-tools
```

The Make dry run should invoke `bin/tool-updater` or `bin/tool-updater.exe`, not `go run`.

## Troubleshooting

- **Windows reports an elevation error for `go run`.** Use `make update-tools`, or build and run `bin/tool-updater.exe` directly. The Make target avoids Go's temporary executable directory.
- **A checksum is not found.** Check the release asset name, version/tag prefix, checksum-file template, and checksum format together. The updater matches the expanded asset name exactly.
- **Manifest validation fails.** Ensure every checksum-bearing tool defines every platform listed at the manifest's top level, every checksum is 64 lowercase hexadecimal characters, and any `versionFiles` path stays inside the repository.
