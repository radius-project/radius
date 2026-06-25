#!/usr/bin/env bash
set -euo pipefail

# Installs the Bicep CLI into a user-owned directory (no sudo) for the current
# platform. Works on linux and darwin, amd64 and arm64, for both CI and local
# development; under GitHub Actions the install dir is added to the job PATH so
# later steps can run bicep.
#
# This installs the upstream Azure/bicep CLI for running/validating Bicep. It is
# NOT build/install-bicep.sh, which packages Bicep into the Radius container image.
#
# The pinned version and per-platform SHA-256 checksums are normally provided by
# build/tools.mk through the environment. The script is generic, so when a value
# is not supplied it is resolved at runtime:
#   * empty BICEP_VERSION           -> the latest published release
#   * missing checksum for platform -> install without verification (a warning is
#     printed; Azure/bicep publishes no checksums file to fall back to)
#
# Usage: install-bicep.sh [install_dir]
#
# Environment (all optional):
#   BICEP_VERSION                Release tag, e.g. v0.42.1. Empty selects latest.
#   BICEP_CHECKSUM_<OS>_<ARCH>   SHA-256 for that platform (e.g.
#                                BICEP_CHECKSUM_LINUX_AMD64).
#   BICEP_OS / BICEP_ARCH        Override the target platform (default: host).
#                                Used to stage a binary for another architecture,
#                                e.g. the multi-arch bicep container image build.
#                                When the target is not the host, the post-install
#                                run check and PATH export are skipped.
#   BICEP_INSTALL_DIR            Install directory. Default: $HOME/.local/bin.
#   GITHUB_TOKEN                 If set, authenticates GitHub requests (higher
#                                rate limits; required for private repositories).

readonly REPO="Azure/bicep"
readonly RELEASES_URL="https://github.com/${REPO}/releases"

log() { echo "[install-bicep] $*" >&2; }
fail() {
    echo "[install-bicep] ERROR: $*" >&2
    exit 1
}

# Temporary working directory for downloads, removed on exit.
WORKDIR=""
cleanup() {
    [ -n "${WORKDIR:-}" ] && rm -rf "${WORKDIR}"
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
        || fail "could not resolve the latest bicep version"
    printf '%s\n' "${effective_url##*/tag/}"
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
    local install_dir host_os host_arch os arch platform asset checksum version runnable

    command -v curl >/dev/null 2>&1 || fail "curl is required but was not found"

    install_dir="${1:-${BICEP_INSTALL_DIR:-}}"
    [ -n "$install_dir" ] || install_dir="${HOME}/.local/bin"

    # Default to the host platform; BICEP_OS/BICEP_ARCH override the target so the
    # bicep container image build can stage a binary for another architecture.
    host_os="$(detect_os)"
    host_arch="$(detect_arch)"
    os="${BICEP_OS:-$host_os}"
    arch="${BICEP_ARCH:-$host_arch}"
    platform="${os}_${arch}"

    # Map the platform to the bicep release asset and its checksum. Bicep names
    # assets bicep-<os>-<arch> with os in {linux,osx}, arch in {x64,arm64}; it has
    # no linux 32-bit build, so linux/arm falls back to the x64 binary.
    case "$platform" in
        linux_amd64) asset="bicep-linux-x64"; checksum="${BICEP_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) asset="bicep-linux-arm64"; checksum="${BICEP_CHECKSUM_LINUX_ARM64:-}" ;;
        linux_arm) asset="bicep-linux-x64"; checksum="${BICEP_CHECKSUM_LINUX_AMD64:-}" ;;
        darwin_amd64) asset="bicep-osx-x64"; checksum="${BICEP_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) asset="bicep-osx-arm64"; checksum="${BICEP_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) fail "unsupported platform '${platform}'" ;;
    esac

    # The downloaded binary is runnable here only when it targets the host.
    if [ "$os" = "$host_os" ] && [ "$arch" = "$host_arch" ]; then
        runnable=true
    else
        runnable=false
    fi

    # Normalize the requested version: strip whitespace, treat empty as the latest
    # release, and accept a bare number (0.42.1) as well as a tag (v0.42.1).
    version="${BICEP_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [ -z "$version" ]; then
        log "resolving latest bicep version..."
        version="$(resolve_latest_version)"
    elif [ "${version#[0-9]}" != "$version" ]; then
        version="v${version}"
    fi
    [ -n "$version" ] || fail "could not determine the bicep version to install"

    # Skip if already present in the target directory, verifying the version when
    # the binary can run on this host.
    if [ -x "${install_dir}/bicep" ]; then
        if ! $runnable; then
            log "bicep already present at ${install_dir}/bicep"
            return 0
        elif "${install_dir}/bicep" --version 2>/dev/null | grep -q "${version#v}"; then
            log "bicep ${version} already installed: ${install_dir}/bicep"
            return 0
        fi
    fi

    WORKDIR="$(mktemp -d)"

    log "downloading ${asset} ${version}..."
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/${asset}" -o "${WORKDIR}/bicep" \
        || fail "could not download ${asset} ${version}"

    # Azure/bicep does not publish checksums, so verification only happens when a
    # pinned checksum is supplied (the common case via build/tools.mk).
    if [ -n "$checksum" ]; then
        verify_checksum "$checksum" "${WORKDIR}/bicep"
    else
        log "WARNING: no checksum supplied for ${platform}; installing without verification."
    fi
    chmod 0755 "${WORKDIR}/bicep"

    mkdir -p "$install_dir"
    mv "${WORKDIR}/bicep" "${install_dir}/bicep"
    log "installed bicep ${version} (${asset}) to ${install_dir}/bicep"

    # Verify it runs and expose it on PATH only when it targets the host.
    if $runnable; then
        "${install_dir}/bicep" --version >/dev/null 2>&1 \
            || fail "installed bicep failed to run (${install_dir}/bicep)"
        if [ -n "${GITHUB_PATH:-}" ]; then
            echo "$install_dir" >> "$GITHUB_PATH"
        fi
    fi
}

trap cleanup EXIT
main "$@"
