#!/usr/bin/env bash

set -euo pipefail

# Installs the kubectl Kubernetes CLI into a user-owned directory (no sudo) for
# the current platform. Works on linux and darwin, amd64 and arm64, for both CI
# and local development; under GitHub Actions the install dir is added to the job
# PATH so later steps can run kubectl.
#
# kubectl is published on the Kubernetes release CDN (dl.k8s.io), not GitHub. The
# pinned version and per-platform SHA-256 checksums are normally provided by
# build/tools.mk through the environment. The script is generic, so when a value
# is not supplied it is resolved at runtime:
#   * empty KUBECTL_VERSION         -> the latest stable release (stable.txt)
#   * missing checksum for platform -> read from the release's own
#                                      'kubectl.sha256' file
#
# Usage: install-kubectl.sh [install_dir]
#
# Environment (all optional):
#   KUBECTL_VERSION                Release tag, e.g. v1.30.0. Empty selects the
#                                  latest stable release.
#   KUBECTL_CHECKSUM_<OS>_<ARCH>   SHA-256 for that platform (e.g.
#                                  KUBECTL_CHECKSUM_LINUX_AMD64). Empty fetches it
#                                  from the release's published 'kubectl.sha256'.
#   KUBECTL_INSTALL_DIR            Install directory. Default: $HOME/.local/bin.

readonly RELEASE_URL="https://dl.k8s.io/release"

log() { echo "[install-kubectl] $*" >&2; }
fail() {
    echo "[install-kubectl] ERROR: $*" >&2
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

# curl wrapper: enforces HTTPS + TLS 1.2 and sets a User-Agent. dl.k8s.io is a
# public CDN, so no authentication is required.
dl_curl() {
    curl --proto '=https' --tlsv1.2 -H "User-Agent: kubectl-installer" "$@"
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

# Resolve the latest stable release tag from the release channel's stable.txt.
resolve_latest_version() {
    dl_curl -fsSL "${RELEASE_URL}/stable.txt" \
        || fail "could not resolve the latest stable kubectl version"
}

# Print the SHA-256 of the kubectl binary, read from the release's own published
# 'kubectl.sha256' file (which contains just the hash).
checksum_from_release() {
    local version="$1" os="$2" arch="$3"
    dl_curl -fsSL "${RELEASE_URL}/${version}/bin/${os}/${arch}/kubectl.sha256" -o "${WORKDIR}/kubectl.sha256" \
        || fail "could not download kubectl.sha256 for ${version} ${os}/${arch}"
    awk 'NR == 1 { print $1 }' "${WORKDIR}/kubectl.sha256"
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
    local install_dir os arch platform version checksum

    command -v curl >/dev/null 2>&1 || fail "curl is required but was not found"

    install_dir="${1:-${KUBECTL_INSTALL_DIR:-}}"
    [ -n "$install_dir" ] || install_dir="${HOME}/.local/bin"

    os="$(detect_os)"
    arch="$(detect_arch)"
    platform="${os}_${arch}"

    # Normalize the requested version: strip whitespace, treat empty as the
    # latest stable release, and accept a bare number (1.30.0) as well as a tag.
    version="${KUBECTL_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [ -z "$version" ]; then
        log "resolving latest stable kubectl version..."
        version="$(resolve_latest_version)"
    elif [ "${version#[0-9]}" != "$version" ]; then
        version="v${version}"
    fi
    [ -n "$version" ] || fail "could not determine the kubectl version to install"

    if command -v kubectl >/dev/null 2>&1 && kubectl version --client 2>/dev/null | grep -q "${version#v}"; then
        log "kubectl ${version} already installed: $(command -v kubectl)"
        return 0
    fi

    WORKDIR="$(mktemp -d)"

    # Expected checksum: prefer the value supplied for this platform, otherwise
    # read it from the release's own published 'kubectl.sha256' file.
    case "$platform" in
        linux_amd64) checksum="${KUBECTL_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${KUBECTL_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${KUBECTL_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${KUBECTL_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) checksum="" ;;
    esac
    if [ -z "$checksum" ]; then
        log "no checksum supplied for ${platform}; reading it from the ${version} release..."
        checksum="$(checksum_from_release "$version" "$os" "$arch")"
    fi
    [ -n "$checksum" ] || fail "could not determine the SHA-256 checksum for kubectl ${version} ${platform}"

    log "downloading kubectl ${version} (${os}/${arch})..."
    dl_curl -fsSL "${RELEASE_URL}/${version}/bin/${os}/${arch}/kubectl" -o "${WORKDIR}/kubectl" \
        || fail "could not download kubectl ${version} for ${os}/${arch}"
    verify_checksum "$checksum" "${WORKDIR}/kubectl"
    chmod 0755 "${WORKDIR}/kubectl"

    mkdir -p "$install_dir"
    mv "${WORKDIR}/kubectl" "${install_dir}/kubectl"
    "${install_dir}/kubectl" version --client >/dev/null 2>&1 \
        || fail "installed kubectl failed to run (${install_dir}/kubectl)"
    log "installed kubectl ${version} to ${install_dir}/kubectl"

    # Make kubectl available to later GitHub Actions steps.
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$install_dir" >> "$GITHUB_PATH"
    fi
}

trap cleanup EXIT
main "$@"
