#!/usr/bin/env bash

set -euo pipefail

# Installs GoReleaser into a user-owned directory for the current platform.
# The pinned version and SHA-256 checksums normally come from build/tools.yaml
# through build/tools.generated.mk.

readonly REPO="goreleaser/goreleaser"
readonly RELEASES_URL="https://github.com/${REPO}/releases"

WORKDIR=""

log() { echo "[install-goreleaser] $*" >&2; }
fail() {
    echo "[install-goreleaser] ERROR: $*" >&2
    exit 1
}

cleanup() {
    if [[ -n "${WORKDIR:-}" && -d "${WORKDIR}" ]]; then
        rm -rf "${WORKDIR}"
    fi
}

gh_curl() {
    local headers=(-H "User-Agent: goreleaser-installer")
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
    )" || fail "could not resolve the latest GoReleaser version"
    printf '%s\n' "${effective_url##*/tag/}"
}

checksum_from_release() {
    local version="$1"
    local asset="$2"
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/checksums.txt" \
        -o "${WORKDIR}/checksums.txt" ||
        fail "could not download checksums.txt for ${version}"
    awk -v asset="${asset}" '$2 == asset { print $1 }' \
        "${WORKDIR}/checksums.txt"
}

verify_checksum() {
    local expected="$1"
    local file="$2"
    if command -v sha256sum >/dev/null 2>&1; then
        echo "${expected}  ${file}" | sha256sum -c - >/dev/null
    elif command -v shasum >/dev/null 2>&1; then
        echo "${expected}  ${file}" | shasum -a 256 -c - >/dev/null
    else
        fail "neither sha256sum nor shasum is available"
    fi
}

main() {
    local install_dir os arch asset_os asset_arch
    local platform version asset checksum

    command -v curl >/dev/null 2>&1 || fail "curl is required"
    command -v tar >/dev/null 2>&1 || fail "tar is required"

    install_dir="${1:-${GORELEASER_INSTALL_DIR:-}}"
    [[ -n "${install_dir}" ]] || install_dir="${HOME}/.local/bin"

    case "$(uname -s)" in
        Linux)
            os="linux"
            asset_os="Linux"
            ;;
        Darwin)
            os="darwin"
            asset_os="Darwin"
            ;;
        *) fail "unsupported OS '$(uname -s)' (supported: Linux, Darwin)" ;;
    esac

    case "$(uname -m)" in
        x86_64 | amd64)
            arch="amd64"
            asset_arch="x86_64"
            ;;
        aarch64 | arm64)
            arch="arm64"
            asset_arch="arm64"
            ;;
        *) fail "unsupported architecture '$(uname -m)'" ;;
    esac
    platform="${os}_${arch}"

    version="${GORELEASER_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [[ -z "${version}" ]]; then
        log "resolving latest GoReleaser version..."
        version="$(resolve_latest_version)"
    elif [[ "${version}" =~ ^[0-9] ]]; then
        version="v${version}"
    fi
    [[ -n "${version}" ]] || fail "could not determine the version to install"

    if command -v goreleaser >/dev/null 2>&1 &&
        goreleaser --version 2>/dev/null | grep -q "${version#v}"; then
        log "GoReleaser ${version} already installed: $(command -v goreleaser)"
        return 0
    fi

    asset="goreleaser_${asset_os}_${asset_arch}.tar.gz"
    case "${platform}" in
        linux_amd64) checksum="${GORELEASER_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${GORELEASER_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${GORELEASER_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${GORELEASER_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) fail "unsupported platform '${platform}'" ;;
    esac

    WORKDIR="$(mktemp -d)"
    if [[ -z "${checksum}" ]]; then
        log "reading ${asset} checksum from the ${version} release..."
        checksum="$(checksum_from_release "${version}" "${asset}")"
    fi
    [[ -n "${checksum}" ]] ||
        fail "could not determine the SHA-256 checksum for ${asset}"

    log "downloading ${asset} ${version}..."
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/${asset}" \
        -o "${WORKDIR}/${asset}" ||
        fail "could not download ${asset} ${version}"
    verify_checksum "${checksum}" "${WORKDIR}/${asset}"

    tar -xzf "${WORKDIR}/${asset}" -C "${WORKDIR}" ||
        fail "could not extract ${asset}"
    [[ -f "${WORKDIR}/goreleaser" ]] ||
        fail "expected goreleaser binary not found in ${asset}"

    chmod 0755 "${WORKDIR}/goreleaser"
    mkdir -p "${install_dir}"
    mv "${WORKDIR}/goreleaser" "${install_dir}/goreleaser"
    "${install_dir}/goreleaser" --version >/dev/null ||
        fail "installed GoReleaser failed to run"
    log "installed GoReleaser ${version} to ${install_dir}/goreleaser"

    if [[ -n "${GITHUB_PATH:-}" ]]; then
        echo "${install_dir}" >>"${GITHUB_PATH}"
    fi
}

trap cleanup EXIT
main "$@"
