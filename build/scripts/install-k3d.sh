#!/usr/bin/env bash

set -euo pipefail

# Installs the k3d (k3s in Docker) cluster tool into a user-owned directory (no
# sudo) for the current platform. Works on linux and darwin, amd64 and arm64, for
# both CI and local development; under GitHub Actions the install dir is added to
# the job PATH so later steps can run k3d.
#
# k3d is published as a per-platform single binary on k3d-io/k3d's GitHub
# releases. The pinned version and per-platform SHA-256 checksums are normally
# provided by build/tools.mk through the environment. The script is generic, so
# when a value is not supplied it is resolved at runtime:
#   * empty K3D_VERSION             -> the latest published release
#   * missing checksum for platform -> read from the release's own combined
#                                      'checksums.txt' file
#
# Usage: install-k3d.sh [install_dir]
#
# Environment (all optional):
#   K3D_VERSION                 Release tag, e.g. v5.9.0. Empty selects latest.
#   K3D_CHECKSUM_<OS>_<ARCH>    SHA-256 of the binary for that platform (e.g.
#                               K3D_CHECKSUM_LINUX_AMD64). Empty fetches it from
#                               the release's combined 'checksums.txt' file.
#   K3D_INSTALL_DIR             Install directory. Default: $HOME/.local/bin.
#   GITHUB_TOKEN                If set, authenticates GitHub requests (higher rate
#                               limits; required for private repositories).

readonly REPO="k3d-io/k3d"
readonly RELEASES_URL="https://github.com/${REPO}/releases"

log() { echo "[install-k3d] $*" >&2; }
fail() {
    echo "[install-k3d] ERROR: $*" >&2
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
    local headers=(-H "User-Agent: k3d-installer")
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
        || fail "could not resolve the latest k3d version"
    printf '%s\n' "${effective_url##*/tag/}"
}

# Print the SHA-256 of an asset, read from the release's own published combined
# 'checksums.txt' file. k3d lists assets under a '_dist/' path prefix
# ('<sha256>  _dist/k3d-<os>-<arch>'), so compare against the basename.
checksum_from_release() {
    local version="$1" asset="$2"
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/checksums.txt" -o "${WORKDIR}/checksums.txt" \
        || fail "could not download checksums.txt for ${version}"
    awk -v a="$asset" '{ n = $2; sub(/^.*\//, "", n); if (n == a) print $1 }' "${WORKDIR}/checksums.txt"
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

    install_dir="${1:-${K3D_INSTALL_DIR:-}}"
    [ -n "$install_dir" ] || install_dir="${HOME}/.local/bin"

    os="$(detect_os)"
    arch="$(detect_arch)"
    platform="${os}_${arch}"
    asset="k3d-${os}-${arch}"

    # Normalize the requested version: strip whitespace, treat empty as the
    # latest release, and accept a bare number (5.9.0) as well as a tag (v5.9.0).
    version="${K3D_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [ -z "$version" ]; then
        log "resolving latest k3d version..."
        version="$(resolve_latest_version)"
    elif [ "${version#[0-9]}" != "$version" ]; then
        version="v${version}"
    fi
    [ -n "$version" ] || fail "could not determine the k3d version to install"

    if command -v k3d >/dev/null 2>&1 && k3d version 2>/dev/null | grep -q "${version#v}"; then
        log "k3d ${version} already installed: $(command -v k3d)"
        return 0
    fi

    WORKDIR="$(mktemp -d)"

    # Expected checksum: prefer the value supplied for this platform, otherwise
    # read it from the release's own published combined 'checksums.txt' file.
    case "$platform" in
        linux_amd64) checksum="${K3D_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${K3D_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${K3D_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${K3D_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) checksum="" ;;
    esac
    if [ -z "$checksum" ]; then
        log "no checksum supplied for ${platform}; reading it from the ${version} release..."
        checksum="$(checksum_from_release "$version" "$asset")"
    fi
    [ -n "$checksum" ] || fail "could not determine the SHA-256 checksum for ${asset} ${version}"

    log "downloading ${asset} ${version}..."
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/${asset}" -o "${WORKDIR}/k3d" \
        || fail "could not download ${asset} ${version}"
    verify_checksum "$checksum" "${WORKDIR}/k3d"
    chmod 0755 "${WORKDIR}/k3d"

    mkdir -p "$install_dir"
    mv "${WORKDIR}/k3d" "${install_dir}/k3d"
    "${install_dir}/k3d" version >/dev/null 2>&1 \
        || fail "installed k3d failed to run (${install_dir}/k3d)"
    log "installed k3d ${version} to ${install_dir}/k3d"

    # Make k3d available to later GitHub Actions steps.
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$install_dir" >> "$GITHUB_PATH"
    fi
}

trap cleanup EXIT
main "$@"
