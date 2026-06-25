#!/usr/bin/env bash

set -euo pipefail

# Installs the yq YAML processor into a user-owned directory (no sudo) for the
# current platform. Works on linux and darwin, amd64 and arm64, for both CI and
# local development; under GitHub Actions the install dir is added to the job
# PATH so later steps can run yq.
#
# The pinned version and per-platform SHA-256 checksums are normally provided by
# build/tools.mk through the environment. The script is generic, so when a value
# is not supplied it is resolved at runtime:
#   * empty YQ_VERSION              -> the latest published release
#   * missing checksum for platform -> read from the release's own checksums file
#
# Usage: install-yq.sh [install_dir]
#
# Environment (all optional):
#   YQ_VERSION                Release tag, e.g. v4.53.3. Empty selects latest.
#   YQ_CHECKSUM_<OS>_<ARCH>   SHA-256 for that platform (e.g.
#                             YQ_CHECKSUM_LINUX_AMD64). Empty fetches it from the
#                             release's published checksums file.
#   YQ_INSTALL_DIR            Install directory. Default: $HOME/.local/bin.
#   GITHUB_TOKEN              If set, authenticates GitHub requests (higher rate
#                             limits; required for private repositories).

readonly REPO="mikefarah/yq"
readonly RELEASES_URL="https://github.com/${REPO}/releases"

log() { echo "[install-yq] $*" >&2; }
fail() {
    echo "[install-yq] ERROR: $*" >&2
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
        || fail "could not resolve the latest yq version"
    printf '%s\n' "${effective_url##*/tag/}"
}

# Print the SHA-256 of an asset, read from the release's own checksums. yq
# publishes 'checksums' (one row per asset, many hash columns) alongside
# 'checksums_hashes_order' (the algorithm name for each column).
checksum_from_release() {
    local version="$1" asset="$2" order_index
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/checksums_hashes_order" -o "${WORKDIR}/order" \
        || fail "could not download checksums_hashes_order for ${version}"
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/checksums" -o "${WORKDIR}/checksums" \
        || fail "could not download checksums for ${version}"
    order_index="$(grep -n '^SHA-256$' "${WORKDIR}/order" | head -n1 | cut -d: -f1)" \
        || fail "SHA-256 column not found in checksums_hashes_order"
    # Column 1 is the filename; hash N is in column N+1.
    awk -v asset="$asset" -v col="$((order_index + 1))" \
        '$1 == asset { print $col }' "${WORKDIR}/checksums"
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

    install_dir="${1:-${YQ_INSTALL_DIR:-}}"
    [ -n "$install_dir" ] || install_dir="${HOME}/.local/bin"

    os="$(detect_os)"
    arch="$(detect_arch)"
    platform="${os}_${arch}"
    asset="yq_${platform}"

    # Normalize the requested version: strip whitespace, treat empty as the
    # latest release, and accept a bare number (4.53.3) as well as a tag (v4.53.3).
    version="${YQ_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [ -z "$version" ]; then
        log "resolving latest yq version..."
        version="$(resolve_latest_version)"
    elif [ "${version#[0-9]}" != "$version" ]; then
        version="v${version}"
    fi
    [ -n "$version" ] || fail "could not determine the yq version to install"

    if command -v yq >/dev/null 2>&1 && yq --version 2>/dev/null | grep -q "${version#v}"; then
        log "yq ${version} already installed: $(command -v yq)"
        return 0
    fi

    WORKDIR="$(mktemp -d)"

    # Expected checksum: prefer the value supplied for this platform, otherwise
    # read it from the release's own published checksums.
    case "$platform" in
        linux_amd64) checksum="${YQ_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${YQ_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${YQ_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${YQ_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) checksum="" ;;
    esac
    if [ -z "$checksum" ]; then
        log "no checksum supplied for ${platform}; reading it from the ${version} release..."
        checksum="$(checksum_from_release "$version" "$asset")"
    fi
    [ -n "$checksum" ] || fail "could not determine the SHA-256 checksum for ${asset} ${version}"

    log "downloading ${asset} ${version}..."
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/${asset}" -o "${WORKDIR}/yq" \
        || fail "could not download ${asset} ${version}"
    verify_checksum "$checksum" "${WORKDIR}/yq"
    chmod 0755 "${WORKDIR}/yq"

    mkdir -p "$install_dir"
    mv "${WORKDIR}/yq" "${install_dir}/yq"
    "${install_dir}/yq" --version >/dev/null 2>&1 \
        || fail "installed yq failed to run (${install_dir}/yq)"
    log "installed yq ${version} to ${install_dir}/yq"

    # Make yq available to later GitHub Actions steps.
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$install_dir" >> "$GITHUB_PATH"
    fi
}

trap cleanup EXIT
main "$@"
