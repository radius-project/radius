#!/usr/bin/env bash

set -euo pipefail

# Installs the Helm CLI (the 'helm' binary) into a user-owned directory (no sudo)
# for the current platform. Works on linux and darwin, amd64 and arm64, for both
# CI and local development; under GitHub Actions the install dir is added to the
# job PATH so later steps can run helm.
#
# Helm is published as a per-platform tarball on the Helm release CDN
# (get.helm.sh), not GitHub. The pinned version and per-platform SHA-256 checksums
# (of the tarball) are normally provided by build/tools.mk through the
# environment. The script is generic, so when a value is not supplied it is
# resolved at runtime:
#   * empty HELM_VERSION            -> the latest published release
#   * missing checksum for platform -> read from the release's own
#                                      '<tarball>.sha256sum' file
#
# Usage: install-helm.sh [install_dir]
#
# Environment (all optional):
#   HELM_VERSION                Release tag, e.g. v4.2.2. Empty selects latest.
#   HELM_CHECKSUM_<OS>_<ARCH>   SHA-256 of the tarball for that platform (e.g.
#                               HELM_CHECKSUM_LINUX_AMD64). Empty fetches it from
#                               the release's '<tarball>.sha256sum' file.
#   HELM_INSTALL_DIR            Install directory. Default: $HOME/.local/bin.

readonly DOWNLOAD_URL="https://get.helm.sh"
# Used only to resolve the latest tag when HELM_VERSION is empty; binaries always
# come from the get.helm.sh CDN above.
readonly LATEST_URL="https://github.com/helm/helm/releases/latest"

log() { echo "[install-helm] $*" >&2; }
fail() {
    echo "[install-helm] ERROR: $*" >&2
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

# curl wrapper: enforces HTTPS + TLS 1.2 and sets a User-Agent. get.helm.sh is a
# public CDN, so no authentication is required.
helm_curl() {
    curl --proto '=https' --tlsv1.2 -H "User-Agent: helm-installer" "$@"
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
resolve_latest_version() {
    local effective_url
    effective_url="$(helm_curl -fsSLI -o /dev/null -w '%{url_effective}' "${LATEST_URL}")" \
        || fail "could not resolve the latest Helm version"
    printf '%s\n' "${effective_url##*/tag/}"
}

# Print the SHA-256 of the tarball, read from the release's own published
# '<tarball>.sha256sum' file (a single '<sha256>  <tarball>' line).
checksum_from_release() {
    local tarball="$1"
    helm_curl -fsSL "${DOWNLOAD_URL}/${tarball}.sha256sum" -o "${WORKDIR}/${tarball}.sha256sum" \
        || fail "could not download ${tarball}.sha256sum"
    awk 'NR == 1 { print $1 }' "${WORKDIR}/${tarball}.sha256sum"
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
    local install_dir os arch platform tarball version checksum extracted

    command -v curl >/dev/null 2>&1 || fail "curl is required but was not found"
    command -v tar >/dev/null 2>&1 || fail "tar is required but was not found"

    install_dir="${1:-${HELM_INSTALL_DIR:-}}"
    [ -n "$install_dir" ] || install_dir="${HOME}/.local/bin"

    os="$(detect_os)"
    arch="$(detect_arch)"
    platform="${os}_${arch}"

    # Normalize the requested version: strip whitespace, treat empty as the
    # latest release, and accept a bare number (4.2.2) as well as a tag (v4.2.2).
    version="${HELM_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [ -z "$version" ]; then
        log "resolving latest Helm version..."
        version="$(resolve_latest_version)"
    elif [ "${version#[0-9]}" != "$version" ]; then
        version="v${version}"
    fi
    [ -n "$version" ] || fail "could not determine the Helm version to install"

    if command -v helm >/dev/null 2>&1 && helm version --short 2>/dev/null | grep -q "${version#v}"; then
        log "Helm ${version} already installed: $(command -v helm)"
        return 0
    fi

    tarball="helm-${version}-${os}-${arch}.tar.gz"
    WORKDIR="$(mktemp -d)"

    # Expected checksum: prefer the value supplied for this platform, otherwise
    # read it from the release's own published '<tarball>.sha256sum' file.
    case "$platform" in
        linux_amd64) checksum="${HELM_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${HELM_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${HELM_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${HELM_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) checksum="" ;;
    esac
    if [ -z "$checksum" ]; then
        log "no checksum supplied for ${platform}; reading it from the ${version} release..."
        checksum="$(checksum_from_release "$tarball")"
    fi
    [ -n "$checksum" ] || fail "could not determine the SHA-256 checksum for ${tarball}"

    log "downloading ${tarball}..."
    helm_curl -fsSL "${DOWNLOAD_URL}/${tarball}" -o "${WORKDIR}/${tarball}" \
        || fail "could not download ${tarball}"
    verify_checksum "$checksum" "${WORKDIR}/${tarball}"

    # The tarball extracts to a '<os>-<arch>/' directory containing the binary.
    tar -xzf "${WORKDIR}/${tarball}" -C "${WORKDIR}" \
        || fail "could not extract ${tarball}"
    extracted="${WORKDIR}/${os}-${arch}/helm"
    [ -f "$extracted" ] || fail "expected 'helm' binary not found in ${tarball}"
    chmod 0755 "$extracted"

    mkdir -p "$install_dir"
    mv "$extracted" "${install_dir}/helm"
    "${install_dir}/helm" version --short >/dev/null 2>&1 \
        || fail "installed helm failed to run (${install_dir}/helm)"
    log "installed Helm ${version} to ${install_dir}/helm"

    # Make helm available to later GitHub Actions steps.
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$install_dir" >> "$GITHUB_PATH"
    fi
}

trap cleanup EXIT
main "$@"
