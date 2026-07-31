#!/usr/bin/env bash

set -euo pipefail

# Installs the jq JSON processor into a user-owned directory (no sudo) for the
# current platform. Works on linux and darwin, amd64 and arm64, for both CI and
# local development; under GitHub Actions the install dir is added to the job
# PATH so later steps can run jq.
#
# jq is published as a single per-platform binary on jqlang/jq's GitHub releases,
# whose release tags are 'jq-<version>' (e.g. jq-1.8.2) and whose darwin assets
# are named 'macos'. The pinned version and per-platform SHA-256 checksums are
# normally provided by build/tools.yaml through the generated Make include. The script is
# generic, so when a value is not supplied it is resolved at runtime:
#   * empty JQ_VERSION              -> the latest published release
#   * missing checksum for platform -> read from the release's own 'sha256sum.txt'
#
# Usage: install-jq.sh [install_dir]
#
# Environment (all optional):
#   JQ_VERSION                Release version, e.g. 1.8.2 (the bare 'jq-1.8.2' tag
#                             is also accepted). Empty selects the latest release.
#   JQ_CHECKSUM_<OS>_<ARCH>   SHA-256 for that platform (e.g.
#                             JQ_CHECKSUM_LINUX_AMD64). Empty fetches it from the
#                             release's 'sha256sum.txt' file.
#   JQ_INSTALL_DIR            Install directory. Default: $HOME/.local/bin.
#   GITHUB_TOKEN              If set, authenticates GitHub requests (higher rate
#                             limits; required for private repositories).

readonly REPO="jqlang/jq"
readonly RELEASES_URL="https://github.com/${REPO}/releases"

log() { echo "[install-jq] $*" >&2; }
fail() {
    echo "[install-jq] ERROR: $*" >&2
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
    local headers=(-H "User-Agent: jq-installer")
    if [ -n "${GITHUB_TOKEN:-}" ]; then
        headers+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
    fi
    # --retry rides out transient failures (timeouts and HTTP 408/429/5xx such as
    # the 504 gateway timeouts GitHub's release CDN returns intermittently) with
    # exponential backoff, while still failing fast on 404s (a wrong version).
    curl --proto '=https' --tlsv1.2 --retry 5 --retry-connrefused "${headers[@]}" "$@"
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

# Resolve the latest release version by following the /releases/latest redirect
# and stripping the 'jq-' tag prefix. Avoids the GitHub API (no token, no rate
# limit).
resolve_latest_version() {
    local effective_url tag
    effective_url="$(gh_curl -fsSLI -o /dev/null -w '%{url_effective}' "${RELEASES_URL}/latest")" \
        || fail "could not resolve the latest jq version"
    tag="${effective_url##*/tag/}"
    printf '%s\n' "${tag#jq-}"
}

# Print the SHA-256 of an asset, read from the release's own 'sha256sum.txt'
# (standard 'sha256sum' format: '<sha256>  <asset>').
checksum_from_release() {
    local version="$1" asset="$2"
    gh_curl -fsSL "${RELEASES_URL}/download/jq-${version}/sha256sum.txt" -o "${WORKDIR}/sha256sum.txt" \
        || fail "could not download sha256sum.txt for jq-${version}"
    awk -v a="$asset" '$2 == a { print $1 }' "${WORKDIR}/sha256sum.txt"
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
    local install_dir os arch platform asset_os asset version checksum

    command -v curl >/dev/null 2>&1 || fail "curl is required but was not found"

    install_dir="${1:-${JQ_INSTALL_DIR:-}}"
    [ -n "$install_dir" ] || install_dir="${HOME}/.local/bin"

    os="$(detect_os)"
    arch="$(detect_arch)"
    platform="${os}_${arch}"

    # jq names its darwin assets 'macos' (e.g. jq-macos-arm64); linux stays 'linux'.
    case "$os" in
        darwin) asset_os="macos" ;;
        *) asset_os="$os" ;;
    esac
    asset="jq-${asset_os}-${arch}"

    # Normalize the requested version: strip whitespace and any leading 'jq-' tag
    # prefix, and treat empty as the latest release.
    version="${JQ_VERSION:-}"
    version="${version//[[:space:]]/}"
    version="${version#jq-}"
    if [ -z "$version" ]; then
        log "resolving latest jq version..."
        version="$(resolve_latest_version)"
    fi
    [ -n "$version" ] || fail "could not determine the jq version to install"

    if command -v jq >/dev/null 2>&1 && jq --version 2>/dev/null | grep -q "^jq-${version}$"; then
        log "jq ${version} already installed: $(command -v jq)"
        return 0
    fi

    WORKDIR="$(mktemp -d)"

    # Expected checksum: prefer the value supplied for this platform, otherwise
    # read it from the release's own published checksums.
    case "$platform" in
        linux_amd64) checksum="${JQ_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${JQ_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${JQ_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${JQ_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) checksum="" ;;
    esac
    if [ -z "$checksum" ]; then
        log "no checksum supplied for ${platform}; reading it from the jq-${version} release..."
        checksum="$(checksum_from_release "$version" "$asset")"
    fi
    [ -n "$checksum" ] || fail "could not determine the SHA-256 checksum for ${asset} jq-${version}"

    log "downloading ${asset} jq-${version}..."
    gh_curl -fsSL "${RELEASES_URL}/download/jq-${version}/${asset}" -o "${WORKDIR}/jq" \
        || fail "could not download ${asset} jq-${version}"
    verify_checksum "$checksum" "${WORKDIR}/jq"
    chmod 0755 "${WORKDIR}/jq"

    mkdir -p "$install_dir"
    mv "${WORKDIR}/jq" "${install_dir}/jq"
    "${install_dir}/jq" --version >/dev/null 2>&1 \
        || fail "installed jq failed to run (${install_dir}/jq)"
    log "installed jq ${version} to ${install_dir}/jq"

    # Make jq available to later GitHub Actions steps.
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$install_dir" >> "$GITHUB_PATH"
    fi
}

trap cleanup EXIT
main "$@"
