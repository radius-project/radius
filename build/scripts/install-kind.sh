#!/usr/bin/env bash

set -euo pipefail

# Installs the kind (Kubernetes IN Docker) cluster tool into a user-owned
# directory (no sudo) for the current platform. Works on linux and darwin,
# amd64 and arm64, for both CI and local development; under GitHub Actions the
# install dir is added to the job PATH so later steps can run kind.
#
# The pinned version and per-platform SHA-256 checksums are normally provided by
# build/tools.mk through the environment. The script is generic, so when a value
# is not supplied it is resolved at runtime:
#   * empty KIND_VERSION            -> the latest published release
#   * missing checksum for platform -> read from the release's own
#                                      '<asset>.sha256sum' file
#
# Usage: install-kind.sh [install_dir]
#
# Environment (all optional):
#   KIND_VERSION                Release tag, e.g. v0.32.0. Empty selects latest.
#   KIND_CHECKSUM_<OS>_<ARCH>   SHA-256 for that platform (e.g.
#                               KIND_CHECKSUM_LINUX_AMD64). Empty fetches it from
#                               the release's published '<asset>.sha256sum' file.
#   KIND_INSTALL_DIR            Install directory. Default: $HOME/.local/bin.
#   GITHUB_TOKEN                If set, authenticates GitHub requests (higher rate
#                               limits; required for private repositories).

readonly REPO="kubernetes-sigs/kind"
readonly RELEASES_URL="https://github.com/${REPO}/releases"

log() { echo "[install-kind] $*" >&2; }
fail() {
    echo "[install-kind] ERROR: $*" >&2
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
    local headers=(-H "User-Agent: ${REPO##*/}-installer")
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
        || fail "could not resolve the latest kind version"
    printf '%s\n' "${effective_url##*/tag/}"
}

# Print the SHA-256 of an asset, read from the release's own published
# '<asset>.sha256sum' file. kind publishes one such file per binary; each
# contains a single '<sha256>  <asset>' line.
checksum_from_release() {
    local version="$1" asset="$2"
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/${asset}.sha256sum" -o "${WORKDIR}/${asset}.sha256sum" \
        || fail "could not download ${asset}.sha256sum for ${version}"
    awk 'NR == 1 { print $1 }' "${WORKDIR}/${asset}.sha256sum"
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
    local install_dir os arch platform asset version checksum

    command -v curl >/dev/null 2>&1 || fail "curl is required but was not found"

    install_dir="${1:-${KIND_INSTALL_DIR:-}}"
    [ -n "$install_dir" ] || install_dir="${HOME}/.local/bin"

    os="$(detect_os)"
    arch="$(detect_arch)"
    platform="${os}_${arch}"
    asset="kind-${os}-${arch}"

    # Normalize the requested version: strip whitespace, treat empty as the
    # latest release, and accept a bare number (0.32.0) as well as a tag (v0.32.0).
    version="${KIND_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [ -z "$version" ]; then
        log "resolving latest kind version..."
        version="$(resolve_latest_version)"
    elif [ "${version#[0-9]}" != "$version" ]; then
        version="v${version}"
    fi
    [ -n "$version" ] || fail "could not determine the kind version to install"

    if command -v kind >/dev/null 2>&1 && kind --version 2>/dev/null | grep -q "${version#v}"; then
        log "kind ${version} already installed: $(command -v kind)"
        return 0
    fi

    WORKDIR="$(mktemp -d)"

    # Expected checksum: prefer the value supplied for this platform, otherwise
    # read it from the release's own published '<asset>.sha256sum' file.
    case "$platform" in
        linux_amd64) checksum="${KIND_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${KIND_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${KIND_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${KIND_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) checksum="" ;;
    esac
    if [ -z "$checksum" ]; then
        log "no checksum supplied for ${platform}; reading it from the ${version} release..."
        checksum="$(checksum_from_release "$version" "$asset")"
    fi
    [ -n "$checksum" ] || fail "could not determine the SHA-256 checksum for ${asset} ${version}"

    log "downloading ${asset} ${version}..."
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/${asset}" -o "${WORKDIR}/kind" \
        || fail "could not download ${asset} ${version}"
    verify_checksum "$checksum" "${WORKDIR}/kind"
    chmod 0755 "${WORKDIR}/kind"

    mkdir -p "$install_dir"
    mv "${WORKDIR}/kind" "${install_dir}/kind"
    "${install_dir}/kind" version >/dev/null 2>&1 \
        || fail "installed kind failed to run (${install_dir}/kind)"
    log "installed kind ${version} to ${install_dir}/kind"

    # Make kind available to later GitHub Actions steps.
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$install_dir" >> "$GITHUB_PATH"
    fi
}

trap cleanup EXIT
main "$@"
