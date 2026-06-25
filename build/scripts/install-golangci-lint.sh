#!/usr/bin/env bash

set -euo pipefail

# Installs golangci-lint (the 'golangci-lint' binary) into a user-owned directory
# (no sudo) for the current platform. Works on linux and darwin, amd64 and arm64,
# for both CI and local development; under GitHub Actions the install dir is added
# to the job PATH so later steps can run golangci-lint.
#
# golangci-lint is published as a per-platform tarball on golangci/golangci-lint's
# GitHub releases. The pinned version and per-platform SHA-256 checksums (of the
# tarball) are normally provided by build/tools.mk through the environment. The
# script is generic, so when a value is not supplied it is resolved at runtime:
#   * empty GOLANGCI_LINT_VERSION   -> the latest published release
#   * missing checksum for platform -> read from the release's own
#                                      'golangci-lint-<version>-checksums.txt' file
#
# Usage: install-golangci-lint.sh [install_dir]
#
# Environment (all optional):
#   GOLANGCI_LINT_VERSION                Release tag, e.g. v2.12.2. Empty selects
#                                        latest.
#   GOLANGCI_LINT_CHECKSUM_<OS>_<ARCH>   SHA-256 of the tarball for that platform
#                                        (e.g. GOLANGCI_LINT_CHECKSUM_LINUX_AMD64).
#                                        Empty fetches it from the release's
#                                        'golangci-lint-<version>-checksums.txt' file.
#   GOLANGCI_LINT_INSTALL_DIR            Install directory. Default: $HOME/.local/bin.
#   GITHUB_TOKEN                         If set, authenticates GitHub requests
#                                        (higher rate limits; required for private
#                                        repositories).

readonly REPO="golangci/golangci-lint"
readonly RELEASES_URL="https://github.com/${REPO}/releases"

log() { echo "[install-golangci-lint] $*" >&2; }
fail() {
    echo "[install-golangci-lint] ERROR: $*" >&2
    exit 1
}

# Temporary working directory for downloads, removed on exit. Uses an explicit
# 'if' (not '&&') so the function returns 0 when WORKDIR is unset; otherwise the
# failing test would become the EXIT trap's status and abort an otherwise
# successful run, e.g. the early return when the tool is already installed.
WORKDIR=""
cleanup() {
    if [ -n "${WORKDIR:-}" ] && [ -d "${WORKDIR}" ]; then
        rm -rf "${WORKDIR}"
    fi
}

# curl wrapper for GitHub requests: enforces HTTPS + TLS 1.2, sets a User-Agent,
# and adds an Authorization header when GITHUB_TOKEN is set (raises API rate
# limits and allows private repositories). curl drops the Authorization header on
# cross-host redirects, so the token is not sent to the download CDN. The array is
# seeded with the User-Agent so it is never empty -- expanding an empty array
# under 'set -u' is an error on bash 3.2 (macOS).
gh_curl() {
    local headers=(-H "User-Agent: golangci-lint-installer")
    if [ -n "${GITHUB_TOKEN:-}" ]; then
        headers+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
    fi
    curl --proto '=https' --tlsv1.2 "${headers[@]}" "$@"
}

detect_os() {
    case "$(uname -s)" in
        Linux) echo "linux" ;;
        Darwin) echo "darwin" ;;
        *) fail "unsupported OS '$(uname -s)' (supported: Linux, Darwin)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64 | amd64) echo "amd64" ;;
        aarch64 | arm64) echo "arm64" ;;
        *) fail "unsupported architecture '$(uname -m)' (supported: amd64, arm64)" ;;
    esac
}

# Resolve the latest release tag by following the /releases/latest redirect.
# Avoids the GitHub API (no token, no rate limit).
resolve_latest_version() {
    local effective_url
    effective_url="$(gh_curl -fsSLI -o /dev/null -w '%{url_effective}' "${RELEASES_URL}/latest")" \
        || fail "could not resolve the latest golangci-lint version"
    printf '%s\n' "${effective_url##*/tag/}"
}

# Print the SHA-256 of an asset, read from the release's own published combined
# 'golangci-lint-<version>-checksums.txt' file ('<sha256>  <asset>').
checksum_from_release() {
    local version_no_v="$1" asset="$2"
    gh_curl -fsSL "${RELEASES_URL}/download/v${version_no_v}/golangci-lint-${version_no_v}-checksums.txt" -o "${WORKDIR}/checksums.txt" \
        || fail "could not download golangci-lint-${version_no_v}-checksums.txt"
    awk -v a="$asset" '$2 == a { print $1 }' "${WORKDIR}/checksums.txt"
}

verify_checksum() {
    local expected="$1" file="$2"
    if command -v sha256sum >/dev/null 2>&1; then
        echo "${expected}  ${file}" | sha256sum -c - >/dev/null
    elif command -v shasum >/dev/null 2>&1; then
        echo "${expected}  ${file}" | shasum -a 256 -c - >/dev/null
    else
        fail "neither sha256sum nor shasum is available for checksum verification"
    fi
}

main() {
    local install_dir os arch platform version version_no_v asset checksum extracted

    command -v curl >/dev/null 2>&1 || fail "curl is required but was not found"
    command -v tar >/dev/null 2>&1 || fail "tar is required but was not found"

    install_dir="${1:-${GOLANGCI_LINT_INSTALL_DIR:-}}"
    [ -n "$install_dir" ] || install_dir="${HOME}/.local/bin"

    os="$(detect_os)"
    arch="$(detect_arch)"
    platform="${os}_${arch}"

    # Normalize the requested version: strip whitespace, treat empty as the
    # latest release, and accept a bare number (2.12.2) as well as a tag (v2.12.2).
    version="${GOLANGCI_LINT_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [ -z "$version" ]; then
        log "resolving latest golangci-lint version..."
        version="$(resolve_latest_version)"
    elif [ "${version#[0-9]}" != "$version" ]; then
        version="v${version}"
    fi
    [ -n "$version" ] || fail "could not determine the golangci-lint version to install"

    if command -v golangci-lint >/dev/null 2>&1 && golangci-lint version 2>/dev/null | grep -q "${version#v}"; then
        log "golangci-lint ${version} already installed: $(command -v golangci-lint)"
        return 0
    fi

    # The asset and the directory it extracts to embed the version without the
    # leading 'v'.
    version_no_v="${version#v}"
    asset="golangci-lint-${version_no_v}-${os}-${arch}.tar.gz"
    WORKDIR="$(mktemp -d)"

    # Expected checksum: prefer the value supplied for this platform, otherwise
    # read it from the release's own published combined checksums file.
    case "$platform" in
        linux_amd64) checksum="${GOLANGCI_LINT_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${GOLANGCI_LINT_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${GOLANGCI_LINT_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${GOLANGCI_LINT_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) checksum="" ;;
    esac
    if [ -z "$checksum" ]; then
        log "no checksum supplied for ${platform}; reading it from the ${version} release..."
        checksum="$(checksum_from_release "$version_no_v" "$asset")"
    fi
    [ -n "$checksum" ] || fail "could not determine the SHA-256 checksum for ${asset}"

    log "downloading ${asset} ${version}..."
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/${asset}" -o "${WORKDIR}/${asset}" \
        || fail "could not download ${asset} ${version}"
    verify_checksum "$checksum" "${WORKDIR}/${asset}"

    # The tarball extracts to a 'golangci-lint-<version>-<os>-<arch>/' directory
    # containing the binary.
    tar -xzf "${WORKDIR}/${asset}" -C "${WORKDIR}" \
        || fail "could not extract ${asset}"
    extracted="${WORKDIR}/golangci-lint-${version_no_v}-${os}-${arch}/golangci-lint"
    [ -f "$extracted" ] || fail "expected 'golangci-lint' binary not found in ${asset}"
    chmod 0755 "$extracted"

    mkdir -p "$install_dir"
    mv "$extracted" "${install_dir}/golangci-lint"
    "${install_dir}/golangci-lint" version >/dev/null 2>&1 \
        || fail "installed golangci-lint failed to run (${install_dir}/golangci-lint)"
    log "installed golangci-lint ${version} to ${install_dir}/golangci-lint"

    # Make golangci-lint available to later GitHub Actions steps.
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$install_dir" >> "$GITHUB_PATH"
    fi
}

trap cleanup EXIT
main "$@"
