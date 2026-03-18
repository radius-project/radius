## Plan: Migrate Radius Build & Release to GoReleaser

**TL;DR**: Replace the hand-rolled Makefile + Python + shell build/release system with GoReleaser as the single source of truth for compiling Go binaries, building Docker images, creating GitHub Releases, and generating changelogs. This eliminates ~600 lines of Makefile logic (`build/build.mk`, `build/docker.mk`, `build/version.mk`, `build/artifacts.mk`), the Python version parser (`.github/scripts/get_release_version.py`), and the convoluted multi-step manual release process. The release flow becomes: **push a `v*` tag → GoReleaser handles everything**. Multi-repo coordination and non-Go artifacts (Helm chart, Bicep types) stay as lightweight post-release workflow steps.

---

## Current State Analysis

### Binaries Built (7 total from main module)

| Binary | Path | Platforms | Purpose |
|--------|------|-----------|---------|
| `rad` | `cmd/rad` | linux/{amd64,arm,arm64}, darwin/{amd64,arm64}, windows/amd64 | CLI tool |
| `ucpd` | `cmd/ucpd` | linux/{amd64,arm,arm64} | Universal Control Plane daemon |
| `applications-rp` | `cmd/applications-rp` | linux/{amd64,arm,arm64} | Applications Resource Provider |
| `dynamic-rp` | `cmd/dynamic-rp` | linux/{amd64,arm,arm64} | Dynamic Resource Provider |
| `controller` | `cmd/controller` | linux/{amd64,arm,arm64} | Radius Controller |
| `pre-upgrade` | `cmd/pre-upgrade` | linux/{amd64,arm,arm64} | Pre-upgrade hook |
| `docgen` | `cmd/docgen` | linux/amd64 | Doc generation (build-only, not released) |

### Test Binaries (separate go.mod files — cannot use main GoReleaser config)

| Binary | Path | Module |
|--------|------|--------|
| `testrp` | `test/testrp` | `github.com/radius-project/radius/test/testrp` |
| `magpiego` | `test/magpiego` | `github.com/radius-project/radius/test/magpiego` |

### Docker Images (6 production + 1 external + 2 test)

**IMPORTANT: Base images differ per component** — GoReleaser Dockerfiles must match:

| Image | Binary | Base Image | Extra Files |
|-------|--------|-----------|-------------|
| `ghcr.io/radius-project/ucpd` | ucpd | `gcr.io/distroless/static:nonroot` | `deploy/manifest/built-in-providers/self-hosted/*` |
| `ghcr.io/radius-project/applications-rp` | applications-rp | `alpine:3.21.3` (needs ca-certs, git) | — |
| `ghcr.io/radius-project/dynamic-rp` | dynamic-rp | `alpine:3.20` (needs ca-certs, git) | — |
| `ghcr.io/radius-project/controller` | controller | `debian:bullseye-slim` (needs ca-certs, openssl) | — |
| `ghcr.io/radius-project/pre-upgrade` | pre-upgrade | `gcr.io/distroless/static:nonroot` | — |
| `ghcr.io/radius-project/bicep` | *(external download)* | `alpine:3.21.3` | Bicep binary + config |
| `ghcr.io/radius-project/testrp` | testrp | distroless | — |
| `ghcr.io/radius-project/magpiego` | magpiego | distroless | — |

### Version Injection (ldflags into `pkg/version`)

```
-X github.com/radius-project/radius/pkg/version.channel=<CHANNEL>
-X github.com/radius-project/radius/pkg/version.release=<RELEASE>
-X github.com/radius-project/radius/pkg/version.commit=<COMMIT>
-X github.com/radius-project/radius/pkg/version.version=<VERSION>
-X github.com/radius-project/radius/pkg/version.chartVersion=<CHART_VERSION>
```

### Current Pain Points

1. **`versions.yaml` as trigger**: Release workflow watches for `versions.yaml` changes, parses with Python, then creates tags across 4 repos → error-prone indirection
2. **~30-step manual release process**: Multiple repos, manual tag pushes, waiting for workflows, manual verification (see `docs/contributing/contributing-releases/README.md`)
3. **~600 lines of Make includes**: `build.mk` (~160 lines), `docker.mk` (~160 lines), `version.mk`, `artifacts.mk` for what GoReleaser does declaratively
4. **Custom Python parser** (`get_release_version.py`, ~60 lines): Complex semver regex + ref parsing to compute `REL_VERSION`, `REL_CHANNEL`, `CHART_VERSION`
5. **Manual archive/checksum logic**: `cp`/`sha256sum` loop in `publish-release` job
6. **6× matrix build for CLI**: Each CLI platform combo runs a separate workflow job (6 total), each doing `make build` — slow and wasteful
7. **Multi-repo tag coordination**: `release.yaml` checks out 4 repos (radius, recipes, dashboard, bicep-types-aws), runs shell scripts to create tags/branches, dispatches DE image publish via `azure-octo/radius-publisher`
8. **No changelog automation**: Release notes manually written; RC uses `--generate-notes`, official uses hand-written file
9. **Docker image tagging uses `REL_CHANNEL`** (`0.55` or `0.55.0-rc1`), not `REL_VERSION` — GoReleaser `{{ .Version }}` maps cleanly to this

### Current Multi-Platform Docker Build Flow

The existing `docker.mk` uses `docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7` with `COPY ./linux_${TARGETARCH:-amd64}/release/<binary> /` — i.e., the Go cross-compilation output is copied from the dist directory by arch. GoReleaser replaces this entirely: it cross-compiles, then for each `dockers` entry, places the correct binary in the build context automatically.

### Scripts Inventory (`.github/scripts/`)

Scripts to **remove** (replaced by GoReleaser):
- `get_release_version.py` — version parsing → GoReleaser + 5-line shell
- `release-get-version.sh` — finds which version in `versions.yaml` lacks a tag → eliminated (tag push triggers release directly)
- `release-create-tag-and-branch.sh` — creates tag + release branch in a repo → simplified into `release-coordination.yaml`

Scripts to **keep** (unrelated to build/release):
- `monitor-remote-workflow.mjs` — monitors dispatched workflows (used by bicep-types job)
- `release-verification.sh` — post-release verification
- `validate_semver.py` — PR validation
- `changes.mjs`, `codeql-matrix.mjs`, `radius-bot.js`, etc.

---

## Migration Plan

### Step 1: Create `.goreleaser.yaml` — Binary Builds

Define **6 build entries** (skip `docgen` — simple `go build` in Makefile for local use).

```yaml
version: 2

project_name: radius

before:
  hooks:
    - go mod tidy

builds:
  - id: rad
    main: ./cmd/rad
    binary: rad
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64, arm]
    ignore:
      - goos: windows
        goarch: arm
      - goos: windows
        goarch: arm64
      - goos: darwin
        goarch: arm
    ldflags: &ldflags
      - -s -w
      - -X github.com/radius-project/radius/pkg/version.channel={{ .Env.REL_CHANNEL }}
      - -X github.com/radius-project/radius/pkg/version.release={{ .Version }}
      - -X github.com/radius-project/radius/pkg/version.commit={{ .FullCommit }}
      - -X github.com/radius-project/radius/pkg/version.version={{ .Tag }}
      - -X github.com/radius-project/radius/pkg/version.chartVersion={{ .Env.CHART_VERSION }}

  - id: ucpd
    main: ./cmd/ucpd
    binary: ucpd
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64, arm]
    ldflags: *ldflags

  - id: applications-rp
    main: ./cmd/applications-rp
    binary: applications-rp
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64, arm]
    ldflags: *ldflags

  - id: dynamic-rp
    main: ./cmd/dynamic-rp
    binary: dynamic-rp
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64, arm]
    ldflags: *ldflags

  - id: controller
    main: ./cmd/controller
    binary: controller
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64, arm]
    ldflags: *ldflags

  - id: pre-upgrade
    main: ./cmd/pre-upgrade
    binary: pre-upgrade
    env: [CGO_ENABLED=0]
    goos: [linux]
    goarch: [amd64, arm64, arm]
    ldflags: *ldflags
```

**Key improvement over current**: GoReleaser compiles all 6 CLI platform combos + all server binaries in a **single job** with parallel compilation, replacing the 6-job matrix build. GoReleaser derives `Version`, `Tag`, `FullCommit` from git — eliminates `get_release_version.py` and `build/version.mk`.

### Step 2: Archives & Checksums

```yaml
archives:
  # CLI archives for GitHub Release download
  - id: rad-archive
    builds: [rad]
    name_template: "rad_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format: tar.gz
    format_overrides:
      - goos: windows
        format: zip
    files:
      - LICENSE
      - THIRD-PARTY-NOTICES.txt

  # Server binaries: no archive — they go into Docker images only
  - id: server-binaries
    builds: [ucpd, applications-rp, dynamic-rp, controller, pre-upgrade]
    format: binary
    name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: "checksums.txt"
  algorithm: sha256
```

Replaces the manual `sha256sum -b` loop and `cp`/`mkdir` logic in the `publish-release` job.

### Step 3: GoReleaser Dockerfiles (Matching Current Base Images)

GoReleaser places the pre-compiled binary at the root of a temp build context. Create **simplified, single-stage Dockerfiles** for each component that **match the current base images exactly**:

**`deploy/images/ucpd/Dockerfile.goreleaser`** — distroless with manifest files:
```dockerfile
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY ucpd /ucpd
COPY manifest/ /manifest/
USER 65532:65532
ENTRYPOINT ["/ucpd"]
```

**`deploy/images/applications-rp/Dockerfile.goreleaser`** — Alpine (needs ca-certs + git):
```dockerfile
FROM alpine:3.21.3
RUN apk --no-cache add ca-certificates git && \
    addgroup -g 65532 rpuser && \
    adduser -u 65532 -G rpuser -s /bin/sh -D rpuser
WORKDIR /
COPY applications-rp /applications-rp
USER rpuser
EXPOSE 8080
ENTRYPOINT ["/applications-rp"]
```

**`deploy/images/dynamic-rp/Dockerfile.goreleaser`** — Alpine (needs ca-certs + git):
```dockerfile
FROM alpine:3.20
RUN apk --no-cache add ca-certificates git && \
    addgroup -g 65532 rpuser && \
    adduser -u 65532 -G rpuser -s /bin/sh -D rpuser
WORKDIR /
COPY dynamic-rp /dynamic-rp
USER rpuser
EXPOSE 8080
ENTRYPOINT ["/dynamic-rp"]
```

**`deploy/images/controller/Dockerfile.goreleaser`** — Debian (needs ca-certs + openssl):
```dockerfile
FROM debian:bullseye-slim
ENV DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates openssl && \
    update-ca-certificates && \
    rm -rf /var/lib/apt/lists/* && \
    useradd -m -s /bin/bash controlleruser && \
    mkdir -p /home/controlleruser && \
    chown -R controlleruser:controlleruser /home/controlleruser
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
WORKDIR /
COPY controller /controller
USER controlleruser
ENTRYPOINT ["/controller"]
```

**`deploy/images/pre-upgrade/Dockerfile.goreleaser`** — distroless:
```dockerfile
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY pre-upgrade /pre-upgrade
USER 65532:65532
ENTRYPOINT ["/pre-upgrade"]
```

**Why separate `.goreleaser` Dockerfiles?** The original Dockerfiles use `COPY ./linux_${TARGETARCH}/release/<binary>` (relative to the Make `dist/` layout). GoReleaser uses a flat temp context with just the binary. Both must remain functional during the transition.

### Step 4: Docker Image Config in `.goreleaser.yaml`

For each server binary, define **3 docker entries** (amd64, arm64, arm/v7) + **1 docker_manifest** entry. Using YAML anchors to reduce repetition:

```yaml
dockers:
  # --- ucpd (3 arches) ---
  - id: ucpd-amd64
    ids: [ucpd]
    goos: linux
    goarch: amd64
    dockerfile: deploy/images/ucpd/Dockerfile.goreleaser
    use: buildx
    image_templates:
      - "ghcr.io/radius-project/ucpd:{{ .Version }}-amd64"
    build_flag_templates: &oci_labels
      - "--pull"
      - "--platform=linux/amd64"
      - "--label=org.opencontainers.image.created={{.Date}}"
      - "--label=org.opencontainers.image.revision={{.FullCommit}}"
      - "--label=org.opencontainers.image.version={{.Version}}"
      - "--label=org.opencontainers.image.source={{.GitURL}}"
      - "--label=org.opencontainers.image.title={{.ProjectName}}"
    extra_files:
      - deploy/manifest/built-in-providers/self-hosted

  - id: ucpd-arm64
    ids: [ucpd]
    goos: linux
    goarch: arm64
    dockerfile: deploy/images/ucpd/Dockerfile.goreleaser
    use: buildx
    image_templates:
      - "ghcr.io/radius-project/ucpd:{{ .Version }}-arm64"
    build_flag_templates:
      - "--pull"
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.created={{.Date}}"
      - "--label=org.opencontainers.image.revision={{.FullCommit}}"
      - "--label=org.opencontainers.image.version={{.Version}}"
      - "--label=org.opencontainers.image.source={{.GitURL}}"
      - "--label=org.opencontainers.image.title={{.ProjectName}}"
    extra_files:
      - deploy/manifest/built-in-providers/self-hosted

  - id: ucpd-armv7
    ids: [ucpd]
    goos: linux
    goarch: arm
    goarm: "7"
    dockerfile: deploy/images/ucpd/Dockerfile.goreleaser
    use: buildx
    image_templates:
      - "ghcr.io/radius-project/ucpd:{{ .Version }}-armv7"
    build_flag_templates:
      - "--pull"
      - "--platform=linux/arm/v7"
      - "--label=org.opencontainers.image.created={{.Date}}"
      - "--label=org.opencontainers.image.revision={{.FullCommit}}"
      - "--label=org.opencontainers.image.version={{.Version}}"
      - "--label=org.opencontainers.image.source={{.GitURL}}"
      - "--label=org.opencontainers.image.title={{.ProjectName}}"
    extra_files:
      - deploy/manifest/built-in-providers/self-hosted

  # --- applications-rp (3 arches, no extra_files) ---
  # ... (same pattern, using deploy/images/applications-rp/Dockerfile.goreleaser)

  # --- dynamic-rp (3 arches, no extra_files) ---
  # ... (same pattern, using deploy/images/dynamic-rp/Dockerfile.goreleaser)

  # --- controller (3 arches, no extra_files) ---
  # ... (same pattern, using deploy/images/controller/Dockerfile.goreleaser)

  # --- pre-upgrade (3 arches, no extra_files) ---
  # ... (same pattern, using deploy/images/pre-upgrade/Dockerfile.goreleaser)

docker_manifests:
  # --- ucpd ---
  - name_template: "ghcr.io/radius-project/ucpd:{{ .Version }}"
    image_templates:
      - "ghcr.io/radius-project/ucpd:{{ .Version }}-amd64"
      - "ghcr.io/radius-project/ucpd:{{ .Version }}-arm64"
      - "ghcr.io/radius-project/ucpd:{{ .Version }}-armv7"
  - name_template: "ghcr.io/radius-project/ucpd:latest"
    skip_push: auto  # skips for prerelease/RC tags
    image_templates:
      - "ghcr.io/radius-project/ucpd:{{ .Version }}-amd64"
      - "ghcr.io/radius-project/ucpd:{{ .Version }}-arm64"
      - "ghcr.io/radius-project/ucpd:{{ .Version }}-armv7"

  # --- applications-rp ---
  # ... (same pattern)
  # --- dynamic-rp ---
  # ... (same pattern)
  # --- controller ---
  # ... (same pattern)
  # --- pre-upgrade ---
  # ... (same pattern)
```

**Key improvements**:
- Replaces the entire `build/docker.mk` (~160 lines) + Docker build steps in `build.yaml`
- Includes `arm/v7` (matching current `linux/arm/v7` platform support)
- OCI labels applied automatically to every image
- `skip_push: auto` on `:latest` manifests prevents latest push for RC/prerelease tags
- `extra_files` on ucpd entries copies the built-in provider manifests

### Step 5: Release & Changelog

```yaml
release:
  github:
    owner: radius-project
    name: radius
  prerelease: auto
  name_template: "Radius {{ .Tag }}"
  header: |
    ## Radius {{ .Tag }}

    See the [release notes](https://docs.radapp.io/release-notes/{{ .Tag }}/) for details.
  extra_files:
    - glob: ./docs/release-notes/*.md

changelog:
  use: github
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^ci:"
      - "^chore:"
      - "Merge pull request"
      - "Merge branch"
  groups:
    - title: "Breaking Changes"
      regexp: "^.*breaking.*$"
      order: 0
    - title: "Features"
      regexp: "^.*feat.*$"
      order: 1
    - title: "Bug Fixes"
      regexp: "^.*fix.*$"
      order: 2
    - title: "Others"
      order: 999
```

**Current vs new**:
- Current: RC → `gh release create --generate-notes --prerelease`, Official → `gh release create --notes-file docs/release-notes/v{ver}.md`
- New: GoReleaser auto-detects prerelease from tag (`v0.55.0-rc1`), generates grouped changelog, attaches the release notes file. Single code path.

### Step 6: Channel-Based Versioning

Compute `REL_CHANNEL` and `CHART_VERSION` in GitHub Actions workflow before invoking GoReleaser. This replaces the 60-line `get_release_version.py`:

```yaml
- name: Compute release metadata
  run: |
    if [[ "$GITHUB_REF" == refs/tags/v* ]]; then
      TAG="${GITHUB_REF#refs/tags/}"
      VERSION="${TAG#v}"
      if [[ "$VERSION" == *-* ]]; then
        echo "REL_CHANNEL=$VERSION" >> "$GITHUB_ENV"
      else
        echo "REL_CHANNEL=$(echo "$VERSION" | cut -d. -f1-2)" >> "$GITHUB_ENV"
      fi
      echo "CHART_VERSION=$VERSION" >> "$GITHUB_ENV"
    elif [[ "$GITHUB_REF" == refs/pull/* ]]; then
      PR_NUM=$(echo "$GITHUB_REF" | cut -d/ -f3)
      echo "REL_CHANNEL=edge" >> "$GITHUB_ENV"
      echo "CHART_VERSION=0.42.42-pr-${PR_NUM}" >> "$GITHUB_ENV"
    else
      echo "REL_CHANNEL=edge" >> "$GITHUB_ENV"
      echo "CHART_VERSION=0.42.42-dev" >> "$GITHUB_ENV"
    fi
```

### Step 7: New `build.yaml` Workflow

Replace the current ~500-line workflow. Key structural changes:
- **CLI builds**: 6 separate matrix jobs → 1 GoReleaser job (compiles all platforms in parallel)
- **Docker images**: Separate `build-and-push-images` job with Make → GoReleaser `dockers` section in same job
- **Release**: Separate `publish-release` job with `gh release create` → GoReleaser `release` in same job
- **Concurrency**: GoReleaser uses Go's native parallelism; workflow-level is simpler

```yaml
name: Build and Release
on:
  workflow_dispatch:
  push:
    branches: [main, release/*]
    tags: ['v*']
  pull_request:
    branches: [main, features/*, release/*]

permissions: {}

concurrency:
  group: build-${{ github.ref }}-${{ github.event.pull_request.number || github.sha }}
  cancel-in-progress: true

env:
  CONTAINER_REGISTRY: ghcr.io/radius-project

jobs:
  changes:
    name: Changes
    uses: ./.github/workflows/__changes.yml
    permissions:
      contents: read
      pull-requests: read

  # ─── Tag push: full GoReleaser release ───
  release:
    name: Release
    if: github.repository == 'radius-project/radius' && startsWith(github.ref, 'refs/tags/v')
    needs: [changes]
    runs-on: ubuntu-24.04
    timeout-minutes: 60
    permissions:
      contents: write
      packages: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Compute release metadata
        run: |
          TAG="${GITHUB_REF#refs/tags/}"
          VERSION="${TAG#v}"
          if [[ "$VERSION" == *-* ]]; then
            echo "REL_CHANNEL=$VERSION" >> "$GITHUB_ENV"
          else
            echo "REL_CHANNEL=$(echo "$VERSION" | cut -d. -f1-2)" >> "$GITHUB_ENV"
          fi
          echo "CHART_VERSION=$VERSION" >> "$GITHUB_ENV"

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GH_RAD_CI_BOT_PAT }}

  # ─── PR / main push: snapshot build for validation ───
  snapshot:
    name: Snapshot Build
    if: >-
      github.repository == 'radius-project/radius' &&
      !startsWith(github.ref, 'refs/tags/v') &&
      needs.changes.outputs.only_changed != 'true'
    needs: [changes]
    runs-on: ubuntu-24.04
    timeout-minutes: 60
    permissions:
      packages: write
      contents: read
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3

      - name: Compute snapshot metadata
        run: |
          if [[ "$GITHUB_REF" == refs/pull/* ]]; then
            PR_NUM=$(echo "$GITHUB_REF" | cut -d/ -f3)
            echo "REL_CHANNEL=edge" >> "$GITHUB_ENV"
            echo "CHART_VERSION=0.42.42-pr-${PR_NUM}" >> "$GITHUB_ENV"
          else
            echo "REL_CHANNEL=edge" >> "$GITHUB_ENV"
            echo "CHART_VERSION=0.42.42-dev" >> "$GITHUB_ENV"
          fi

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --snapshot --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

      # Upload CLI binaries for functional tests and ORAS push
      - uses: actions/upload-artifact@v4
        with:
          name: rad-cli-binaries
          path: dist/rad_*
          retention-days: 3

      # Save Docker images for PR testing (snapshot doesn't push)
      - name: Save container images
        if: github.event_name == 'pull_request'
        run: |
          mkdir -p dist/images
          for img in ucpd applications-rp dynamic-rp controller pre-upgrade; do
            docker save -o "dist/images/${img}.tar" \
              "ghcr.io/radius-project/${img}:{{ .Version }}-amd64" 2>/dev/null || true
          done

      - uses: actions/upload-artifact@v4
        if: github.event_name == 'pull_request'
        with:
          name: container-images
          path: dist/images/
          retention-days: 1

  # ─── Main push: push latest images + ORAS CLI to GHCR ───
  push-latest:
    name: Push Latest Images
    if: github.ref == 'refs/heads/main' && github.event_name == 'push'
    needs: [snapshot]
    runs-on: ubuntu-24.04
    timeout-minutes: 30
    permissions:
      packages: write
      contents: read
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true

      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push latest images
        run: |
          for component in ucpd applications-rp dynamic-rp controller pre-upgrade; do
            docker buildx build \
              --platform linux/amd64,linux/arm64,linux/arm/v7 \
              --push \
              -t "ghcr.io/radius-project/${component}:latest" \
              -f "deploy/images/${component}/Dockerfile" \
              ./dist/
          done

      # ORAS push for rad CLI (edge distribution)
      - uses: oras-project/setup-oras@v1
        with:
          version: "1.1.0"

      - name: Download CLI artifacts
        uses: actions/download-artifact@v4
        with:
          name: rad-cli-binaries
          path: dist/

      - name: Push rad CLI via ORAS
        run: |
          for os_arch in linux-amd64 linux-arm64 linux-arm darwin-amd64 darwin-arm64 windows-amd64; do
            os=$(echo "$os_arch" | cut -d- -f1)
            arch=$(echo "$os_arch" | cut -d- -f2)
            ext=""
            [[ "$os" == "windows" ]] && ext=".exe"
            binary="dist/rad_*_${os}_${arch}/rad${ext}"
            if ls $binary 1>/dev/null 2>&1; then
              cp $binary ./rad${ext}
              oras push "ghcr.io/radius-project/rad/${os}-${arch}:latest" \
                "./rad${ext}" \
                --annotation "org.opencontainers.image.source=https://github.com/radius-project/radius"
              rm -f ./rad${ext}
            fi
          done

  # ─── Helm chart (tag + main push) ───
  helm-chart:
    name: Helm Chart
    if: >-
      github.repository == 'radius-project/radius' &&
      (startsWith(github.ref, 'refs/tags/v') || (github.ref == 'refs/heads/main' && github.event_name == 'push')) &&
      needs.changes.outputs.only_changed != 'true'
    needs: [changes, release, snapshot]
    # 'release' or 'snapshot' — use if-always pattern:
    if: always() && (needs.release.result == 'success' || needs.snapshot.result == 'success')
    runs-on: ubuntu-24.04
    timeout-minutes: 10
    permissions:
      packages: write
      contents: read
    steps:
      - uses: actions/checkout@v4

      - uses: azure/setup-helm@v4
        with:
          version: v3.19.4

      - name: Compute chart version
        run: |
          if [[ "$GITHUB_REF" == refs/tags/v* ]]; then
            VERSION="${GITHUB_REF#refs/tags/v}"
          else
            VERSION="0.42.42-dev"
          fi
          echo "CHART_VERSION=$VERSION" >> "$GITHUB_ENV"

      - name: Lint, package, and push Helm chart
        run: |
          helm lint deploy/Chart
          helm package deploy/Chart \
            --version "$CHART_VERSION" \
            --app-version "$CHART_VERSION"
          echo "${{ secrets.GITHUB_TOKEN }}" | helm registry login \
            -u "${{ github.actor }}" --password-stdin ghcr.io
          helm push "radius-${CHART_VERSION}.tgz" \
            oci://ghcr.io/radius-project/helm-chart

  # ─── Bicep types dispatch (tag + main push) ───
  bicep-types:
    name: Bicep Types Publish
    if: >-
      github.repository == 'radius-project/radius' &&
      (startsWith(github.ref, 'refs/tags/v') || github.ref == 'refs/heads/main') &&
      needs.changes.outputs.only_changed != 'true'
    needs: [changes]
    runs-on: ubuntu-24.04
    timeout-minutes: 15
    environment: publish-bicep
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4

      - name: Compute release metadata
        run: |
          if [[ "$GITHUB_REF" == refs/tags/v* ]]; then
            VERSION="${GITHUB_REF#refs/tags/v}"
            if [[ "$VERSION" == *-* ]]; then
              echo "REL_CHANNEL=$VERSION" >> "$GITHUB_ENV"
            else
              echo "REL_CHANNEL=$(echo "$VERSION" | cut -d. -f1-2)" >> "$GITHUB_ENV"
            fi
          else
            echo "REL_CHANNEL=edge" >> "$GITHUB_ENV"
          fi

      - name: Get App Token
        uses: actions/create-github-app-token@v3
        id: get-token
        with:
          app-id: ${{ secrets.RADIUS_PUBLISHER_BOT_APP_ID }}
          private-key: ${{ secrets.RADIUS_PUBLISHER_BOT_PRIVATE_KEY }}
          permission-metadata: read
          permission-actions: read
          permission-contents: write
          owner: azure-octo
          repositories: radius-publisher

      - name: Capture dispatch start time
        id: dispatch-start
        run: echo "started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$GITHUB_OUTPUT"

      - name: Repository Dispatch
        uses: peter-evans/repository-dispatch@v3
        with:
          token: ${{ steps.get-token.outputs.token }}
          repository: azure-octo/radius-publisher
          event-type: bicep-types
          client-payload: |-
            {
              "source_repository": "${{ github.repository }}",
              "source_ref": "${{ github.ref }}",
              "source_sha": "${{ github.sha }}",
              "rel_channel": "${{ env.REL_CHANNEL }}",
              "registry_target": "radius"
            }

      - name: Monitor remote workflow
        uses: actions/github-script@v7
        with:
          github-token: ${{ steps.get-token.outputs.token }}
          script: |
            const { default: script } = await import(`${process.env.GITHUB_WORKSPACE}/.github/scripts/monitor-remote-workflow.mjs`)
            await script({context, github, core})
        env:
          INPUT_OWNER: azure-octo
          INPUT_REPO: radius-publisher
          INPUT_WORKFLOW_FILE: publish-bicep-types.yml
          INPUT_DISPATCH_STARTED_AT: ${{ steps.dispatch-start.outputs.started_at }}
          INPUT_MAX_WAIT_SECONDS: "600"
          INPUT_POLL_INTERVAL_SECONDS: "15"

  # ─── Bicep image (non-Go, standalone build) ───
  bicep-image:
    name: Bicep Image
    if: >-
      github.repository == 'radius-project/radius' &&
      (startsWith(github.ref, 'refs/tags/v') || (github.ref == 'refs/heads/main' && github.event_name == 'push')) &&
      needs.changes.outputs.only_changed != 'true'
    needs: [changes]
    runs-on: ubuntu-24.04
    timeout-minutes: 15
    permissions:
      packages: write
      contents: read
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - name: Compute tag
        run: |
          if [[ "$GITHUB_REF" == refs/tags/v* ]]; then
            echo "IMG_TAG=${GITHUB_REF#refs/tags/v}" >> "$GITHUB_ENV"
          else
            echo "IMG_TAG=latest" >> "$GITHUB_ENV"
          fi
      - name: Build bicep binaries
        run: bash build/install-bicep.sh
      - name: Build and push Bicep image
        run: |
          docker buildx build \
            --platform linux/amd64,linux/arm64 \
            --push \
            -t "ghcr.io/radius-project/bicep:${IMG_TAG}" \
            -f deploy/images/bicep/Dockerfile \
            ./dist/

  # ─── Test images (functional tests only, PR + main push) ───
  test-images:
    name: Test Images
    if: >-
      github.repository == 'radius-project/radius' &&
      !startsWith(github.ref, 'refs/tags/v') &&
      needs.changes.outputs.only_changed != 'true'
    needs: [changes]
    runs-on: ubuntu-24.04
    timeout-minutes: 15
    permissions:
      packages: write
      contents: read
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - name: Build and push test images
        run: make build-push-test-images

  # ─── Build summary (PR status gate) ───
  build-summary:
    name: Build Summary
    if: always()
    needs: [snapshot, release, bicep-types]
    runs-on: ubuntu-24.04
    permissions: {}
    steps:
      - name: Check results
        run: |
          echo "## Build Results" >> "$GITHUB_STEP_SUMMARY"
          failed=0
          for result in \
            "snapshot:${{ needs.snapshot.result }}" \
            "release:${{ needs.release.result }}" \
            "bicep-types:${{ needs.bicep-types.result }}"; do
            job="${result%%:*}"
            status="${result##*:}"
            if [[ "$status" == "success" || "$status" == "skipped" ]]; then
              echo "- ✅ $job: $status" >> "$GITHUB_STEP_SUMMARY"
            else
              echo "- ❌ $job: $status" >> "$GITHUB_STEP_SUMMARY"
              failed=1
            fi
          done
          exit $failed
```

### Step 8: New `release-coordination.yaml` Workflow

Triggered by **GitHub Release publication** (replaces the current `release.yaml` that watches `versions.yaml`):

```yaml
name: Release Coordination
on:
  release:
    types: [published]

permissions: {}

jobs:
  release-sibling-repos:
    name: Tag sibling repositories
    runs-on: ubuntu-24.04
    timeout-minutes: 10
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4

      - name: Get App Token
        uses: actions/create-github-app-token@v3
        id: get-token
        with:
          app-id: ${{ secrets.RADIUS_PUBLISHER_BOT_APP_ID }}
          private-key: ${{ secrets.RADIUS_PUBLISHER_BOT_PRIVATE_KEY }}
          permission-contents: write
          owner: radius-project
          repositories: recipes,dashboard

      - name: Compute release metadata
        id: meta
        run: |
          TAG="${{ github.event.release.tag_name }}"
          VERSION="${TAG#v}"
          BRANCH="release/$(echo "$VERSION" | cut -d. -f1-2)"
          echo "tag=$TAG" >> "$GITHUB_OUTPUT"
          echo "branch=$BRANCH" >> "$GITHUB_OUTPUT"

      - name: Tag and branch radius-project/recipes
        env:
          GH_TOKEN: ${{ steps.get-token.outputs.token }}
        run: |
          gh api repos/radius-project/recipes/git/refs \
            -f ref="refs/tags/${{ steps.meta.outputs.tag }}" \
            -f sha="$(gh api repos/radius-project/recipes/git/ref/heads/main -q .object.sha)"

      - name: Tag and branch radius-project/dashboard
        env:
          GH_TOKEN: ${{ steps.get-token.outputs.token }}
        run: |
          gh api repos/radius-project/dashboard/git/refs \
            -f ref="refs/tags/${{ steps.meta.outputs.tag }}" \
            -f sha="$(gh api repos/radius-project/dashboard/git/ref/heads/main -q .object.sha)"

  publish-de-image:
    name: Publish Deployment Engine image
    runs-on: ubuntu-24.04
    timeout-minutes: 15
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4

      - name: Get App Token
        uses: actions/create-github-app-token@v3
        id: get-token
        with:
          app-id: ${{ secrets.RADIUS_PUBLISHER_BOT_APP_ID }}
          private-key: ${{ secrets.RADIUS_PUBLISHER_BOT_PRIVATE_KEY }}
          permission-actions: read
          permission-contents: write
          owner: azure-octo
          repositories: radius-publisher

      - name: Capture dispatch start time
        id: dispatch-start
        run: echo "started_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$GITHUB_OUTPUT"

      - name: Dispatch DE image publish
        uses: peter-evans/repository-dispatch@v3
        with:
          token: ${{ steps.get-token.outputs.token }}
          repository: azure-octo/radius-publisher
          event-type: de-image
          client-payload: |-
            {
              "source_repository": "${{ github.repository }}",
              "tag": "${{ github.event.release.tag_name }}"
            }

      - name: Monitor remote workflow
        uses: actions/github-script@v7
        with:
          github-token: ${{ steps.get-token.outputs.token }}
          script: |
            const { default: script } = await import(`${process.env.GITHUB_WORKSPACE}/.github/scripts/monitor-remote-workflow.mjs`)
            await script({context, github, core})
        env:
          INPUT_OWNER: azure-octo
          INPUT_REPO: radius-publisher
          INPUT_WORKFLOW_FILE: publish-de-image.yml
          INPUT_DISPATCH_STARTED_AT: ${{ steps.dispatch-start.outputs.started_at }}
          INPUT_MAX_WAIT_SECONDS: "600"
          INPUT_POLL_INTERVAL_SECONDS: "15"

  trigger-downstream:
    name: Trigger downstream repos
    needs: [release-sibling-repos, publish-de-image]
    runs-on: ubuntu-24.04
    timeout-minutes: 5
    permissions:
      contents: read
    steps:
      - name: Get App Token
        uses: actions/create-github-app-token@v3
        id: get-token
        with:
          app-id: ${{ secrets.RADIUS_PUBLISHER_BOT_APP_ID }}
          private-key: ${{ secrets.RADIUS_PUBLISHER_BOT_PRIVATE_KEY }}
          permission-contents: read
          owner: radius-project
          repositories: docs,samples

      - name: Trigger docs release
        uses: peter-evans/repository-dispatch@v3
        with:
          token: ${{ steps.get-token.outputs.token }}
          repository: radius-project/docs
          event-type: release
          client-payload: '{"tag": "${{ github.event.release.tag_name }}"}'

      - name: Trigger samples test
        uses: peter-evans/repository-dispatch@v3
        with:
          token: ${{ steps.get-token.outputs.token }}
          repository: radius-project/samples
          event-type: release
          client-payload: '{"tag": "${{ github.event.release.tag_name }}"}'

  update-versions-yaml:
    name: Update versions.yaml (documentation)
    needs: [release-sibling-repos]
    runs-on: ubuntu-24.04
    timeout-minutes: 10
    permissions:
      contents: write
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
        with:
          ref: main

      - name: Update versions.yaml and create PR
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          TAG="${{ github.event.release.tag_name }}"
          VERSION="${TAG#v}"
          CHANNEL="$(echo "$VERSION" | cut -d. -f1-2)"

          # Add new version to supported list (uses yq)
          yq -i ".supported = [{\"channel\": \"${CHANNEL}\", \"version\": \"${TAG}\"}] + .supported" versions.yaml

          git checkout -b "auto/update-versions-${VERSION}"
          git add versions.yaml
          git commit -m "chore: update versions.yaml for ${TAG}"
          git push origin "auto/update-versions-${VERSION}"
          gh pr create \
            --title "chore: update versions.yaml for ${TAG}" \
            --body "Auto-generated PR to update versions.yaml after release ${TAG}." \
            --base main
```

### Step 9: Simplify `release.yaml` — Tag Helper

**Delete the current `release.yaml`** (which watches `versions.yaml`, checks out 4 repos, runs shell scripts). Replace with a simple `workflow_dispatch` tag-push helper:

```yaml
name: Create Release Tag
on:
  workflow_dispatch:
    inputs:
      version:
        description: 'Release version (e.g., 0.56.0 or 0.56.0-rc1)'
        required: true
      ref:
        description: 'Git ref to tag (default: main)'
        required: false
        default: main

permissions: {}

jobs:
  create-tag:
    name: Create and push tag
    runs-on: ubuntu-24.04
    timeout-minutes: 5
    environment: release
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ inputs.ref }}
          fetch-depth: 0

      - name: Validate version
        run: python ./.github/scripts/validate_semver.py "${{ inputs.version }}"

      - name: Ensure release branch exists
        run: |
          BRANCH="release/$(echo "${{ inputs.version }}" | cut -d. -f1-2)"
          if ! git ls-remote --heads origin "refs/heads/${BRANCH}" | grep -q "${BRANCH}"; then
            echo "Creating release branch ${BRANCH}..."
            git checkout -b "${BRANCH}"
            git push origin "${BRANCH}"
            git checkout "${{ inputs.ref }}"
          fi

      - name: Create and push tag
        run: |
          git tag "v${{ inputs.version }}"
          git push origin "v${{ inputs.version }}"
```

### Step 10: Handle `versions.yaml` Demotion

- **Keep `versions.yaml`** as documentation/reference only
- **Remove it as a release trigger** — tag push triggers release directly
- **Auto-update via PR** after each release (Step 8 `update-versions-yaml` job)
- No changes to the file format — just remove the `release.yaml` workflow that watches it

### Step 11: Slim Down Makefile

**Remove includes:**
- `build/build.mk` — binary compilation → `goreleaser build`
- `build/docker.mk` — Docker images → `goreleaser release`
- `build/version.mk` — version variables → GoReleaser git introspection
- `build/artifacts.mk` — artifact saving → GoReleaser dist/

**Keep includes (unchanged):**
- `build/help.mk` — help target
- `build/util.mk` — utility functions
- `build/generate.mk` — code generation
- `build/test.mk` — testing
- `build/install.mk` — local install helpers
- `build/prettier.mk` — formatting
- `build/recipes.mk` — recipe publish
- `build/db.mk` — database helpers
- `build/debug.mk` — debug helpers
- `build/workflow.mk` — workflow utilities (enable/disable, not build-critical)

**Updated root Makefile:**
```makefile
ARROW := \033[34;1m=>\033[0m

include build/help.mk build/util.mk build/generate.mk build/test.mk \
        build/recipes.mk build/install.mk build/db.mk build/prettier.mk \
        build/debug.mk build/workflow.mk

##@ Build (GoReleaser)

.PHONY: build
build: ## Build all binaries for current platform
	goreleaser build --single-target --snapshot --clean

.PHONY: build-rad
build-rad: ## Build rad CLI for current platform
	goreleaser build --single-target --snapshot --clean --id rad

.PHONY: build-images
build-images: ## Build all Docker images locally (snapshot)
	goreleaser release --snapshot --clean --skip=publish

.PHONY: build-docgen
build-docgen: ## Build docgen tool
	CGO_ENABLED=0 go build -o ./dist/docgen ./cmd/docgen

##@ Test Images (separate go.mod)

.PHONY: build-test-images
build-test-images: ## Build test container images
	cd test/testrp && CGO_ENABLED=0 go build -o ../../dist/testrp .
	cd test/magpiego && CGO_ENABLED=0 go build -o ../../dist/magpiego .
	docker build -t ghcr.io/radius-project/testrp:latest -f deploy/images/testrp/Dockerfile ./dist/
	docker build -t ghcr.io/radius-project/magpiego:latest -f deploy/images/magpiego/Dockerfile ./dist/

.PHONY: build-push-test-images
build-push-test-images: build-test-images ## Build and push test container images
	docker push ghcr.io/radius-project/testrp:latest
	docker push ghcr.io/radius-project/magpiego:latest
```

### Step 12: ORAS CLI Distribution

Moved into the `push-latest` job (Step 7). On **main push**, after snapshot build completes, push all `rad` platform binaries via ORAS with the `latest` tag. This exactly replicates the current per-platform ORAS push in the `build-and-push-cli` job.

### Step 13: Update Release Documentation

Rewrite `docs/contributing/contributing-releases/README.md`:

**New Release Process (RC):**
1. Run the **Create Release Tag** workflow with version `x.y.z-rc1`
   - This creates the release branch `release/x.y` (if new) and pushes the tag
2. GoReleaser automatically builds everything and creates the GitHub Release (marked as prerelease)
3. `release-coordination.yaml` tags sibling repos and triggers downstream
4. Run the **Release Verification** workflow from the release branch
5. If verification passes, proceed to final release. If not, fix and create `-rc2`.

**New Release Process (Final):**
1. Create a PR adding release notes to `docs/release-notes/vx.y.z.md`
2. Merge the PR to `main`
3. Cherry-pick the release notes commit to the release branch: `git cherry-pick -x <hash>`
4. Run the **Create Release Tag** workflow with version `x.y.z`
5. GoReleaser creates the official GitHub Release with the release notes file attached
6. `release-coordination.yaml` handles all downstream coordination automatically
7. Verify: Check GitHub Release page and workflow runs

**Patch Release:**
1. Merge bug fix to `main`, cherry-pick to `release/x.y`
2. Run the **Create Release Tag** workflow with version `x.y.(z+1)` from the release branch
3. GoReleaser handles everything

Reduces **~30 manual steps → 4-5 steps**.

---

## Files to Create

| File | Purpose |
|------|---------|
| `.goreleaser.yaml` | Main GoReleaser config (builds, archives, dockers, release, changelog) |
| `deploy/images/ucpd/Dockerfile.goreleaser` | GoReleaser Dockerfile — distroless + manifest files |
| `deploy/images/applications-rp/Dockerfile.goreleaser` | GoReleaser Dockerfile — Alpine with ca-certs + git |
| `deploy/images/dynamic-rp/Dockerfile.goreleaser` | GoReleaser Dockerfile — Alpine with ca-certs + git |
| `deploy/images/controller/Dockerfile.goreleaser` | GoReleaser Dockerfile — Debian with ca-certs + openssl |
| `deploy/images/pre-upgrade/Dockerfile.goreleaser` | GoReleaser Dockerfile — distroless |
| `.github/workflows/release-coordination.yaml` | Post-release multi-repo coordination |

## Files to Remove

| File | Replaced By |
|------|-------------|
| `build/build.mk` (~160 lines) | GoReleaser `builds` section |
| `build/docker.mk` (~160 lines) | GoReleaser `dockers` section |
| `build/version.mk` (~10 lines) | GoReleaser git tag introspection |
| `build/artifacts.mk` (~60 lines) | GoReleaser `dist/` directory |
| `.github/scripts/get_release_version.py` (~60 lines) | 15-line shell block in workflow |
| `.github/scripts/release-get-version.sh` (~80 lines) | Direct tag push (no version lookup needed) |
| `.github/scripts/release-create-tag-and-branch.sh` (~40 lines) | `release-coordination.yaml` + tag helper workflow |

## Files to Significantly Rewrite

| File | Changes |
|------|---------|
| `.github/workflows/build.yaml` | Replace with GoReleaser-based workflow (Step 7) |
| `.github/workflows/release.yaml` | Replace with tag helper workflow_dispatch (Step 9) |
| `Makefile` | Remove build/docker/version/artifacts includes, add GoReleaser targets (Step 11) |
| `docs/contributing/contributing-releases/README.md` | Simplified 4-5 step process (Step 13) |

## Files to Keep Unchanged

| File | Reason |
|------|--------|
| `build/workflow.mk` + `build/workflow.sh` | Dev utility for enabling/disabling workflows — not build-critical |
| `.github/scripts/monitor-remote-workflow.mjs` | Used by bicep-types job to track dispatched workflows |
| `.github/scripts/release-verification.sh` | Post-release verification (still useful) |
| `.github/scripts/validate_semver.py` | PR validation + tag helper |
| `.github/workflows/release-verification.yaml` | Manual verification workflow |
| All other `.github/workflows/*.yaml` | Unrelated to build/release (tests, lint, bots, etc.) |
| `deploy/images/*/Dockerfile` (originals) | Keep for local `docker build` during transition |
| `versions.yaml` | Kept as documentation, auto-updated post-release |

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| GoReleaser OSS vs Pro | **OSS** | Split/merge (Pro) not critical; single-runner build is ~15 min |
| Test images (`testrp`, `magpiego`) | **Stay in Makefile** | Separate `go.mod` files, CI-only artifacts |
| Bicep image | **Standalone Docker build** | External binary download, not a Go build |
| `versions.yaml` | **Demoted to docs-only** | Direct tag push triggers release; auto-PR updates it |
| ORAS CLI push | **In `push-latest` job** | 10-line loop, runs on main push only |
| Dockerfiles | **Separate `.goreleaser` variants** | Different base images per component; originals kept for local dev |
| `docgen` binary | **Excluded from GoReleaser** | Build-only tool, never released |
| Arm (32-bit) images | **Keep `arm/v7`** | Current setup builds linux/arm/v7; maintain parity |
| `build/workflow.mk` | **Keep** | Dev utility, not part of build pipeline |
| Release branch creation | **In tag helper workflow** | Creates `release/x.y` if it doesn't exist when tagging |
| `release-coordination.yaml` trigger | **`release: published`** | Fires after GoReleaser creates the GH Release, not on tag push |

## Verification Strategy

1. **Local**: `goreleaser release --snapshot --clean` — builds all binaries + Docker images
2. **CI (PR)**: Snapshot build succeeds, Docker images saved as artifacts, functional tests pass
3. **CI (main push)**: Snapshot build + push latest images + ORAS CLI push
4. **CI (tag push)**: Full GoReleaser release creates GitHub Release with archives + checksums, pushes versioned Docker images to GHCR
5. **CI (release published)**: `release-coordination.yaml` tags sibling repos, dispatches DE image, triggers docs/samples, auto-PRs versions.yaml
6. **Rollback**: Old Makefile targets + original Dockerfiles remain during transition. Feature-flag with `USE_GORELEASER=1` env var in CI if needed.

## Migration Order

1. Create `.goreleaser.yaml` and GoReleaser Dockerfiles (Steps 1-5, 3)
2. Validate locally: `goreleaser release --snapshot --clean`
3. Create new `build.yaml` workflow on feature branch (Step 7)
4. Run CI to verify snapshot builds work end-to-end
5. Slim down Makefile (Step 11)
6. Create `release-coordination.yaml` (Step 8)
7. Replace `release.yaml` with tag helper (Step 9)
8. Create RC tag on feature branch to test full release flow
9. Clean up: remove `build/{build,docker,version,artifacts}.mk`, Python scripts
10. Update release docs (Step 13)
11. Remove old Dockerfiles after 1 release cycle confirms stability
