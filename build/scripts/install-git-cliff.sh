#!/usr/bin/env bash

set -euo pipefail

# Installs git-cliff into a user-owned directory for the current platform.
# The pinned version and SHA-256 checksums normally come from build/tools.yaml
# through build/tools.generated.mk.

readonly REPO="orhun/git-cliff"
readonly RELEASES_URL="https://github.com/${REPO}/releases"

WORKDIR=""

log() { echo "[install-git-cliff] $*" >&2; }
fail() {
    echo "[install-git-cliff] ERROR: $*" >&2
    exit 1
}

cleanup() {
    if [[ -n "${WORKDIR:-}" && -d "${WORKDIR}" ]]; then
        rm -rf "${WORKDIR}"
    fi
}

gh_curl() {
    local headers=(-H "User-Agent: git-cliff-installer")
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
    )" || fail "could not resolve the latest git-cliff version"
    printf '%s\n' "${effective_url##*/tag/}"
}

verify_checksum() {
    local algorithm="$1"
    local expected="$2"
    local file="$3"
    if command -v "${algorithm}sum" >/dev/null 2>&1; then
        echo "${expected}  ${file}" | "${algorithm}sum" -c - >/dev/null
    elif command -v shasum >/dev/null 2>&1; then
        echo "${expected}  ${file}" |
            shasum -a "${algorithm#sha}" -c - >/dev/null
    else
        fail "neither ${algorithm}sum nor shasum is available"
    fi
}

main() {
    local install_dir os arch target platform version version_no_v
    local asset checksum expected_sha512 binary

    command -v curl >/dev/null 2>&1 || fail "curl is required"
    command -v tar >/dev/null 2>&1 || fail "tar is required"

    install_dir="${1:-${GIT_CLIFF_INSTALL_DIR:-}}"
    [[ -n "${install_dir}" ]] || install_dir="${HOME}/.local/bin"

    case "$(uname -s)" in
        Linux) os="linux" ;;
        Darwin) os="darwin" ;;
        *) fail "unsupported OS '$(uname -s)' (supported: Linux, Darwin)" ;;
    esac

    case "$(uname -m)" in
        x86_64 | amd64) arch="amd64" ;;
        aarch64 | arm64) arch="arm64" ;;
        *) fail "unsupported architecture '$(uname -m)'" ;;
    esac
    platform="${os}_${arch}"

    case "${platform}" in
        linux_amd64) target="x86_64-unknown-linux-gnu" ;;
        linux_arm64) target="aarch64-unknown-linux-gnu" ;;
        darwin_amd64) target="x86_64-apple-darwin" ;;
        darwin_arm64) target="aarch64-apple-darwin" ;;
        *) fail "unsupported platform '${platform}'" ;;
    esac

    version="${GIT_CLIFF_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [[ -z "${version}" ]]; then
        log "resolving latest git-cliff version..."
        version="$(resolve_latest_version)"
    elif [[ "${version}" =~ ^[0-9] ]]; then
        version="v${version}"
    fi
    [[ -n "${version}" ]] || fail "could not determine the version to install"

    if command -v git-cliff >/dev/null 2>&1 &&
        [[ "$(git-cliff --version 2>/dev/null | awk '{ print $2; exit }')" == "${version#v}" ]]; then
        log "git-cliff ${version} already installed: $(command -v git-cliff)"
        return 0
    fi

    version_no_v="${version#v}"
    asset="git-cliff-${version_no_v}-${target}.tar.gz"
    case "${platform}" in
        linux_amd64) checksum="${GIT_CLIFF_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${GIT_CLIFF_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${GIT_CLIFF_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${GIT_CLIFF_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) fail "unsupported platform '${platform}'" ;;
    esac

    WORKDIR="$(mktemp -d)"
    log "downloading ${asset} ${version}..."
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/${asset}" \
        -o "${WORKDIR}/${asset}" ||
        fail "could not download ${asset} ${version}"

    if [[ -n "${checksum}" ]]; then
        verify_checksum sha256 "${checksum}" "${WORKDIR}/${asset}"
    else
        log "no SHA-256 supplied; verifying the upstream SHA-512 checksum..."
        gh_curl -fsSL \
            "${RELEASES_URL}/download/${version}/${asset}.sha512" \
            -o "${WORKDIR}/${asset}.sha512" ||
            fail "could not download ${asset}.sha512 ${version}"
        expected_sha512="$(awk 'NR == 1 { print $1 }' \
            "${WORKDIR}/${asset}.sha512")"
        [[ -n "${expected_sha512}" ]] ||
            fail "could not read the upstream SHA-512 checksum"
        verify_checksum sha512 "${expected_sha512}" "${WORKDIR}/${asset}"
    fi

    tar -xzf "${WORKDIR}/${asset}" -C "${WORKDIR}" ||
        fail "could not extract ${asset}"
    binary="$(find "${WORKDIR}" -type f -name git-cliff -print -quit)"
    [[ -n "${binary}" ]] || fail "git-cliff binary not found in ${asset}"

    chmod 0755 "${binary}"
    mkdir -p "${install_dir}"
    mv "${binary}" "${install_dir}/git-cliff"
    "${install_dir}/git-cliff" --version >/dev/null ||
        fail "installed git-cliff failed to run"
    log "installed git-cliff ${version} to ${install_dir}/git-cliff"

    if [[ -n "${GITHUB_PATH:-}" ]]; then
        echo "${install_dir}" >>"${GITHUB_PATH}"
    fi
}

trap cleanup EXIT
main "$@"
