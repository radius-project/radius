# External tool updates with Updatecli

## Purpose

Updatecli manages the command-line tool versions and platform SHA-256 checksums in [`build/tools.yaml`](../build/tools.yaml). Native HTTP sources, JSON/text transformers, YAML/file targets, and file templates replace the custom Go updater. Installation remains the responsibility of the existing `make install-<tool>` targets.

The committed [`build/tools.generated.mk`](../build/tools.generated.mk) supplies pinned metadata to installers and builds. Ordinary Make targets do not invoke Updatecli or regenerate this include automatically.

## Prerequisites

- Install the Updatecli version pinned in [`.updatecli-version`](../.updatecli-version), using the [official installation instructions](https://www.updatecli.io/docs/prologue/installation/) or its versioned release archive.
- Keep GNU Make available for the convenience targets. Go is only required for the fixture tests, not to run the updater.
- Set `UPDATECLI_GITHUB_TOKEN`, `GITHUB_TOKEN`, or `GH_TOKEN` when authenticated GitHub release requests are needed. Tokens are sent only to the configured `https://api.github.com/` endpoints, not to vendor checksum hosts.

On Windows, `C:\Windows\System32\updatecli.exe` is an unrelated Windows utility. Make therefore defaults to the open-source executable in `bin\updatecli-<VERSION>\updatecli.exe`, where `<VERSION>` is the v-prefixed tag in `.updatecli-version`. Extract the matching Windows release archive there, or override `UPDATECLI` with the full path to the executable you installed:

```powershell
make check-tools UPDATECLI="C:\Tools\Updatecli\updatecli.exe"
```

On Linux and macOS, Make defaults to `updatecli` on `PATH`. The GitHub workflows install the pinned version with the upstream setup action.

## Steps

### Preview, apply, or synchronize

| Command                  | Behavior                                                                                                    |
|--------------------------|-------------------------------------------------------------------------------------------------------------|
| `make check-tools`       | Query upstream releases and preview changes without writing pinned metadata.                                |
| `make update-tools`      | Refresh versions and checksums, synchronize consumers, and render Make metadata locally. No commits or PRs. |
| `make generate-tools`    | Synchronize consumers and render Make metadata from existing pins, without network access.                  |
| `make publish-tools`     | Run the native publishing pipeline with the configured GitHub App token.                                    |
| `make test-update-tools` | Exercise the pipelines against local release fixtures.                                                      |

Run `make generate-tools` after manually changing the manifest. Commit the manifest, generated include, and affected version consumers together.

The CLI can also be invoked directly. For example, on Windows:

```powershell
$version = (Get-Content .updatecli-version -Raw).Trim()
& ".\bin\updatecli-$version\updatecli.exe" --disable-version-check --unique-tmp-dir pipeline diff --config .updatecli\update-tools.yaml.tpl --values build\tools.yaml --disable-changelog --disable-udash-report
```

Use `pipeline apply` instead of `pipeline diff` to apply changes. Add `--values-inline "refresh: false"` for offline synchronization.

### Understand the local pipeline

[`update-tools.yaml.tpl`](update-tools.yaml.tpl) expands the inventory into native Updatecli resources. It retains the configured GitHub release, Kubernetes stable-text, and HashiCorp Checkpoint endpoints.

[`version.tpl`](version.tpl) selects a strictly newer semantic version, or keeps the current version when an update is disabled or upstream advertises an older version. There is no release-age filtering in this PoC. The release endpoints determine the stable release channel; the updater does not enumerate release history.

Updatecli's general runtime references cannot execute the Sprig functions used by file templates. Version selection therefore uses native file-template targets and exposes their results to dependent sources through `pipeline` references. Applied runs store these planning files under ignored `bin/updatecli/`; previews use the computed results without needing those files to exist.

All selected versions, required platform checksums, and consumer version snapshots are resolved before targets write pinned metadata. Missing, duplicate, or malformed GitHub asset digests are errors. Consumer conditions also prevent writes when a declared version field has changed since it was read. Writes to several local files are not a filesystem transaction.

[`tools.mk.tpl`](tools.mk.tpl) renders the Make include from the resolved values, not a stale copy of the manifest. Frozen versions still have their current-version checksums refreshed.

### Declare tools and their consumers

The inventory retains `schemaVersion: 1`, a `platforms` list, and a `tools` list. Each tool declares `name`, `makePrefix`, `version`, `source`, and `checksumSource`. Optional `update: false` freezes its version; `notes` records the rationale.

| Source type            | Version value                                                                    |
|------------------------|----------------------------------------------------------------------------------|
| `github-release`       | `tag_name` from `source.latestURL`, with an optional `source.tagPrefix` removed. |
| `stable-text`          | The version string returned by `source.latestURL`.                               |
| `hashicorp-checkpoint` | `current_version` from `source.latestURL`.                                       |

| Checksum type          | Behavior                                                                                                                                          |
|------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| `github-release-asset` | Select exactly one asset by name from the GitHub release for the selected tag and read its `sha256:` digest.                                      |
| `url-file`             | Fetch `checksumSource.urlTemplate`; use `format: first` for a per-asset hash or `format: standard` to select a filename from a checksum manifest. |
| `none`                 | No downloaded binary hashes. Delve records `integrity: go-sumdb` and is installed through Go modules.                                             |

Every checksum-bearing tool must declare an asset for every platform in the inventory. Asset names and checksum URLs can use `{repository}`, `{tag}`, `{version}`, `{version_no_v}`, `{os}`, and `{arch}`; checksum URLs can additionally use `{asset}`. Platform entries may override `os` and `arch`. Stored hashes are 64 lowercase hexadecimal characters.

GitHub asset digests replace the old publisher-specific checksum parsers and download-and-hash paths. There is no fallback to downloading an artifact and trusting a newly calculated hash. A missing digest blocks the update instead of weakening the requirement.

| `versionFiles` format | Required fields            | Behavior                                                              |
|-----------------------|----------------------------|-----------------------------------------------------------------------|
| `plain`               | `path`                     | Write the selected version and a newline.                             |
| `replace`             | `path`, `prefix`, `suffix` | Replace a version between literal markers at the beginning of a line. |
| `yaml`                | `path`, `key`              | Update the exact YAML path with a native YAML target.                 |

Terraform demonstrates all three formats: `.terraform-version`, its Go fallback, and `$.global.terraform.version` in the Helm chart values.

### Publish through GitHub

The [workflow](../.github/workflows/update-tools.yaml) is manual-only for the PoC. It runs fixture coverage, previews real updates, and only publishes when the `publish` input is selected on `main` in `radius-project/radius`. Forks and feature branches remain preview-only.

[`publish-tools.yaml.tpl`](publish-tools.yaml.tpl) uses a native GitHub SCM and pull-request action. Its single shell target invokes the local Updatecli pipeline and observes the checksums of all tracked outputs. The outer target commits only after the inner command succeeds, so per-field targets cannot individually publish an incomplete update. Unchanged tracked files do not create a PR even if an ignored planning file changed.

Publishing requires `UPDATECLI_GITHUB_TOKEN` with repository contents and pull-request write permissions, plus `UPDATECLI_COMMITTER` in `Name <email>` form for the DCO footer. The workflow supplies these from the existing GitHub App. API-created commits are enabled, PRs are drafts, and automatic merging is disabled. The publishing wrapper uses `/bin/sh`; the workflow runs it on Linux.

PR descriptions use Updatecli's native summary and links to upstream release pages instead of a custom per-version Markdown renderer.

## Verification

```sh
make test-update-tools
make generate-tools
git diff -- build/tools.yaml build/tools.generated.mk .terraform-version pkg/recipes/terraform/version.go deploy/Chart/values.yaml
```

The fixtures cover all 14 tools and 52 platform hashes, upgrades without a release-age delay, frozen versions, downgrades, unchanged releases, missing/duplicate/invalid metadata, offline generation, consumer synchronization, and idempotence.

## Troubleshooting

- **Windows runs the wrong program or exits without output.** Use the project-local executable or an explicit `UPDATECLI` path, not the Windows system utility.
- **The CLI rejects a template option.** Install the version in `.updatecli-version`; older releases may not support the native file-template features.
- **A GitHub asset has no digest or the name is ambiguous.** Confirm the configured asset name and upstream metadata. The pipeline intentionally does not guess or skip the platform.
- **A version consumer is missing or its marker moved.** Restore the file or update its `versionFiles` declaration. Prefer a native YAML key for structured YAML consumers.
- **Manual edits did not reach an installer or build.** Run `make generate-tools` and commit the resulting include and consumer changes.
- **GitHub requests are rate-limited.** Supply one of the supported token environment variables.
- **A run fails after local file writes started.** Review the local diff before retrying. Native file targets are not a multi-file transaction; the publishing wrapper prevents a failed inner run from being committed.
