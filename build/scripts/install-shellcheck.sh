#!/usr/bin/env bash

set -euo pipefail

# Installs ShellCheck (the 'shellcheck' binary) into a user-owned directory (no
# sudo) for the current platform. Works on linux and darwin, amd64 and arm64, for
# both CI and local development; under GitHub Actions the install dir is added to
# the job PATH so later steps can run shellcheck.
#
# ShellCheck is published as a per-platform '.tar.xz' archive on
# koalaman/shellcheck's GitHub releases. The pinned version and per-platform
# SHA-256 checksums (of the archive) are normally provided by build/tools.mk
# through the environment. The script is generic, so when a value is not supplied
# it is resolved at runtime:
#   * empty SHELLCHECK_VERSION      -> the latest published release
#   * missing checksum for platform -> install without verification (a warning is
#     printed; koalaman/shellcheck publishes no checksums file to fall back to)
#
# Usage: install-shellcheck.sh [install_dir]
#
# Environment (all optional):
#   SHELLCHECK_VERSION                Release tag, e.g. v0.11.0. Empty selects
#                                     latest.
#   SHELLCHECK_CHECKSUM_<OS>_<ARCH>   SHA-256 of the archive for that platform
#                                     (e.g. SHELLCHECK_CHECKSUM_LINUX_AMD64).
#   SHELLCHECK_INSTALL_DIR            Install directory. Default: $HOME/.local/bin.
#   GITHUB_TOKEN                      If set, authenticates GitHub requests
#                                     (higher rate limits; required for private
#                                     repositories).

readonly REPO="koalaman/shellcheck"
readonly RELEASES_URL="https://github.com/${REPO}/releases"

log() { echo "[install-shellcheck] $*" >&2; }
fail() {
    echo "[install-shellcheck] ERROR: $*" >&2
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
        || fail "could not resolve the latest shellcheck version"
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
    local install_dir os arch platform sc_arch version asset checksum extracted

    command -v curl >/dev/null 2>&1 || fail "curl is required but was not found"
    command -v tar >/dev/null 2>&1 || fail "tar is required but was not found"

    install_dir="${1:-${SHELLCHECK_INSTALL_DIR:-}}"
    [ -n "$install_dir" ] || install_dir="${HOME}/.local/bin"

    os="$(detect_os)"
    arch="$(detect_arch)"
    platform="${os}_${arch}"

    # ShellCheck names its arch x86_64/aarch64 rather than amd64/arm64.
    case "$arch" in
        amd64) sc_arch="x86_64" ;;
        arm64) sc_arch="aarch64" ;;
        *) fail "unsupported architecture '${arch}'" ;;
    esac

    # Normalize the requested version: strip whitespace, treat empty as the latest
    # release, and accept a bare number (0.11.0) as well as a tag (v0.11.0).
    version="${SHELLCHECK_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [ -z "$version" ]; then
        log "resolving latest shellcheck version..."
        version="$(resolve_latest_version)"
    elif [ "${version#[0-9]}" != "$version" ]; then
        version="v${version}"
    fi
    [ -n "$version" ] || fail "could not determine the shellcheck version to install"

    if command -v shellcheck >/dev/null 2>&1 && shellcheck --version 2>/dev/null | grep -q "${version#v}"; then
        log "shellcheck ${version} already installed: $(command -v shellcheck)"
        return 0
    fi

    # The asset and the directory it extracts to embed the version with the
    # leading 'v'.
    asset="shellcheck-${version}.${os}.${sc_arch}.tar.xz"
    WORKDIR="$(mktemp -d)"

    # Expected checksum: prefer the value supplied for this platform, otherwise
    # install without verification (koalaman/shellcheck publishes no checksums
    # file to fall back to).
    case "$platform" in
        linux_amd64) checksum="${SHELLCHECK_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${SHELLCHECK_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${SHELLCHECK_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${SHELLCHECK_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) checksum="" ;;
    esac

    log "downloading ${asset} ${version}..."
    gh_curl -fsSL "${RELEASES_URL}/download/${version}/${asset}" -o "${WORKDIR}/${asset}" \
        || fail "could not download ${asset} ${version}"

    if [ -n "$checksum" ]; then
        verify_checksum "$checksum" "${WORKDIR}/${asset}"
    else
        log "WARNING: no checksum supplied for ${platform}; installing without verification."
    fi

    # The archive extracts to a 'shellcheck-<version>/' directory containing the
    # binary.
    tar -xJf "${WORKDIR}/${asset}" -C "${WORKDIR}" \
        || fail "could not extract ${asset}"
    extracted="${WORKDIR}/shellcheck-${version}/shellcheck"
    [ -f "$extracted" ] || fail "expected 'shellcheck' binary not found in ${asset}"
    chmod 0755 "$extracted"

    mkdir -p "$install_dir"
    mv "$extracted" "${install_dir}/shellcheck"
    "${install_dir}/shellcheck" --version >/dev/null 2>&1 \
        || fail "installed shellcheck failed to run (${install_dir}/shellcheck)"
    log "installed shellcheck ${version} to ${install_dir}/shellcheck"

    # Make shellcheck available to later GitHub Actions steps.
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$install_dir" >> "$GITHUB_PATH"
    fi
}

trap cleanup EXIT
main "$@"
