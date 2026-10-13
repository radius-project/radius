#!/usr/bin/env bash

set -euo pipefail

# Installs Syft into a user-owned directory for the current platform.

readonly REPO="anchore/syft"
readonly RELEASES_URL="https://github.com/${REPO}/releases"

WORKDIR=""

log() { echo "[install-syft] $*" >&2; }
fail() {
    echo "[install-syft] ERROR: $*" >&2
    exit 1
}

cleanup() {
    if [[ -n "${WORKDIR:-}" && -d "${WORKDIR}" ]]; then
        rm -rf "${WORKDIR}"
    fi
}

gh_curl() {
    local headers=(-H "User-Agent: syft-installer")
    if [[ -n "${GITHUB_TOKEN:-}" ]]; then
        headers+=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
    fi
    curl --proto '=https' --tlsv1.2 --retry 5 --retry-connrefused \
        "${headers[@]}" "$@"
}

resolve_latest_version() {
    local effective_url
    effective_url="$(
        gh_curl -fsSLI -o /dev/null -w '%{url_effective}' \
            "${RELEASES_URL}/latest"
    )" || fail "could not resolve the latest Syft version"
    printf '%s\n' "${effective_url##*/tag/}"
}

checksum_from_release() {
    local version="$1"
    local asset="$2"
    local version_no_v="${version#v}"
    local checksum_url

    checksum_url="${RELEASES_URL}/download/${version}/"
    checksum_url+="syft_${version_no_v}_checksums.txt"
    if ! gh_curl -fsSL "${checksum_url}" \
        -o "${WORKDIR}/checksums.txt"; then
        fail "could not download checksums for ${version}"
    fi
    awk -v asset="${asset}" '$2 == asset { print $1 }' \
        "${WORKDIR}/checksums.txt"
}

verify_checksum() {
    local expected="$1"
    local file="$2"

    if command -v sha256sum > /dev/null 2>&1; then
        echo "${expected}  ${file}" | sha256sum -c - > /dev/null
    elif command -v shasum > /dev/null 2>&1; then
        echo "${expected}  ${file}" | shasum -a 256 -c - > /dev/null
    else
        fail "neither sha256sum nor shasum is available"
    fi
}

installed_version() {
    syft version 2> /dev/null | awk '/^Version:/ { print $2; exit }'
}

main() {
    local install_dir
    local os
    local arch
    local platform
    local version
    local version_no_v
    local asset
    local checksum
    local actual_version
    local version_output

    command -v curl > /dev/null 2>&1 || fail "curl is required"
    command -v tar > /dev/null 2>&1 || fail "tar is required"

    install_dir="${1:-${SYFT_INSTALL_DIR:-}}"
    [[ -n "${install_dir}" ]] || install_dir="${HOME}/.local/bin"

    case "$(uname -s)" in
        Linux) os="linux" ;;
        Darwin) os="darwin" ;;
        *)
            fail "unsupported OS '$(uname -s)' (supported: Linux, Darwin)"
            ;;
    esac
    case "$(uname -m)" in
        x86_64 | amd64) arch="amd64" ;;
        aarch64 | arm64) arch="arm64" ;;
        *) fail "unsupported architecture '$(uname -m)'" ;;
    esac
    platform="${os}_${arch}"

    version="${SYFT_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [[ -z "${version}" ]]; then
        log "resolving latest Syft version..."
        version="$(resolve_latest_version)"
    elif [[ "${version}" =~ ^[0-9] ]]; then
        version="v${version}"
    fi
    [[ -n "${version}" ]] || fail "could not determine the version to install"

    if command -v syft > /dev/null 2>&1; then
        if [[ "$(installed_version)" == "${version#v}" ]]; then
            log "Syft ${version} already installed: $(command -v syft)"
            return 0
        fi
    fi

    version_no_v="${version#v}"
    asset="syft_${version_no_v}_${os}_${arch}.tar.gz"
    case "${platform}" in
        linux_amd64) checksum="${SYFT_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${SYFT_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${SYFT_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${SYFT_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) fail "unsupported platform '${platform}'" ;;
    esac

    WORKDIR="$(mktemp -d)"
    if [[ -z "${checksum}" ]]; then
        log "reading ${asset} checksum from the ${version} release..."
        checksum="$(checksum_from_release "${version}" "${asset}")"
    fi
    if [[ ! "${checksum}" =~ ^[0-9a-f]{64}$ ]]; then
        fail "could not determine the SHA-256 checksum for ${asset}"
    fi

    log "downloading ${asset} ${version}..."
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/${asset}" \
        -o "${WORKDIR}/${asset}" || fail "could not download ${asset}"
    verify_checksum "${checksum}" "${WORKDIR}/${asset}"
    if ! tar -xzf "${WORKDIR}/${asset}" -C "${WORKDIR}"; then
        fail "could not extract ${asset}"
    fi
    [[ -f "${WORKDIR}/syft" ]] || fail "expected syft binary not found"

    chmod 0755 "${WORKDIR}/syft"
    mkdir -p "${install_dir}"
    mv "${WORKDIR}/syft" "${install_dir}/syft"
    version_output="$("${install_dir}/syft" version 2> /dev/null)"
    actual_version="$(awk '/^Version:/ { print $2; exit }' \
        <<< "${version_output}")"
    if [[ "${actual_version}" != "${version_no_v}" ]]; then
        fail "installed Syft version does not match ${version}"
    fi
    log "installed Syft ${version} to ${install_dir}/syft"

    if [[ -n "${GITHUB_PATH:-}" ]]; then
        echo "${install_dir}" >> "${GITHUB_PATH}"
    fi
}

trap cleanup EXIT
main "$@"
