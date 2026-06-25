#!/usr/bin/env bash

set -euo pipefail

# Installs the Terraform CLI (the 'terraform' binary) into a user-owned directory
# (no sudo) for the current platform. Works on linux and darwin, amd64 and arm64,
# for both CI and local development; under GitHub Actions the install dir is added
# to the job PATH so later steps can run terraform.
#
# Terraform is published as a per-platform zip on the HashiCorp release CDN
# (releases.hashicorp.com), not GitHub. The pinned version and per-platform
# SHA-256 checksums (of the zip) are normally provided by build/tools.mk through
# the environment. The script is generic, so when a value is not supplied it is
# resolved at runtime:
#   * empty TERRAFORM_VERSION       -> the latest published release
#   * missing checksum for platform -> read from the release's own
#                                      'terraform_<version>_SHA256SUMS' file
#
# Usage: install-terraform.sh [install_dir]
#
# Environment (all optional):
#   TERRAFORM_VERSION                Release, e.g. v1.14.9. Empty selects latest.
#   TERRAFORM_CHECKSUM_<OS>_<ARCH>   SHA-256 of the zip for that platform (e.g.
#                                    TERRAFORM_CHECKSUM_LINUX_AMD64). Empty fetches
#                                    it from the release's
#                                    'terraform_<version>_SHA256SUMS' file.
#   TERRAFORM_INSTALL_DIR            Install directory. Default: $HOME/.local/bin.

readonly DOWNLOAD_URL="https://releases.hashicorp.com/terraform"
# Used only to resolve the latest version when TERRAFORM_VERSION is empty;
# binaries always come from the releases.hashicorp.com CDN above.
readonly CHECKPOINT_URL="https://checkpoint-api.hashicorp.com/v1/check/terraform"

log() { echo "[install-terraform] $*" >&2; }
fail() {
    echo "[install-terraform] ERROR: $*" >&2
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

# curl wrapper: enforces HTTPS + TLS 1.2 and sets a User-Agent.
# releases.hashicorp.com is a public CDN, so no authentication is required.
tf_curl() {
    curl --proto '=https' --tlsv1.2 -H "User-Agent: terraform-installer" "$@"
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

# Resolve the latest stable version from the HashiCorp checkpoint API, whose JSON
# response contains '"current_version":"<version>"'.
resolve_latest_version() {
    local body version
    body="$(tf_curl -fsSL "${CHECKPOINT_URL}")" \
        || fail "could not resolve the latest Terraform version"

    version="$(printf '%s\n' "$body" | grep -o '"current_version":"[^"]*"' | head -n1 | cut -d'"' -f4 || true)"
    [ -n "$version" ] || fail "could not parse the latest Terraform version from the checkpoint response"

    printf '%s\n' "$version"
}

# Print the SHA-256 of the zip, read from the release's own published
# 'terraform_<version>_SHA256SUMS' file ('<sha256>  <asset>').
checksum_from_release() {
    local version_no_v="$1" asset="$2"
    tf_curl -fsSL "${DOWNLOAD_URL}/${version_no_v}/terraform_${version_no_v}_SHA256SUMS" -o "${WORKDIR}/SHA256SUMS" \
        || fail "could not download terraform_${version_no_v}_SHA256SUMS"
    awk -v a="$asset" '$2 == a { print $1 }' "${WORKDIR}/SHA256SUMS"
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
    local install_dir os arch platform version version_no_v asset checksum

    command -v curl >/dev/null 2>&1 || fail "curl is required but was not found"
    command -v unzip >/dev/null 2>&1 || fail "unzip is required but was not found"

    install_dir="${1:-${TERRAFORM_INSTALL_DIR:-}}"
    [ -n "$install_dir" ] || install_dir="${HOME}/.local/bin"

    os="$(detect_os)"
    arch="$(detect_arch)"
    platform="${os}_${arch}"

    # Normalize the requested version: strip whitespace, treat empty as the
    # latest release, and accept a tag (v1.14.9) as well as a bare number
    # (1.14.9, the form used by .terraform-version and the HashiCorp CDN).
    version="${TERRAFORM_VERSION:-}"
    version="${version//[[:space:]]/}"
    if [ -z "$version" ]; then
        log "resolving latest Terraform version..."
        version="$(resolve_latest_version)"
    elif [ "${version#[0-9]}" != "$version" ]; then
        version="v${version}"
    fi
    [ -n "$version" ] || fail "could not determine the Terraform version to install"

    if command -v terraform >/dev/null 2>&1 && terraform version 2>/dev/null | grep -q "${version#v}"; then
        log "Terraform ${version} already installed: $(command -v terraform)"
        return 0
    fi

    # The asset name and CDN path embed the version without the leading 'v'.
    version_no_v="${version#v}"
    asset="terraform_${version_no_v}_${os}_${arch}.zip"
    WORKDIR="$(mktemp -d)"

    # Expected checksum: prefer the value supplied for this platform, otherwise
    # read it from the release's own published 'terraform_<version>_SHA256SUMS' file.
    case "$platform" in
        linux_amd64) checksum="${TERRAFORM_CHECKSUM_LINUX_AMD64:-}" ;;
        linux_arm64) checksum="${TERRAFORM_CHECKSUM_LINUX_ARM64:-}" ;;
        darwin_amd64) checksum="${TERRAFORM_CHECKSUM_DARWIN_AMD64:-}" ;;
        darwin_arm64) checksum="${TERRAFORM_CHECKSUM_DARWIN_ARM64:-}" ;;
        *) checksum="" ;;
    esac
    if [ -z "$checksum" ]; then
        log "no checksum supplied for ${platform}; reading it from the ${version} release..."
        checksum="$(checksum_from_release "$version_no_v" "$asset")"
    fi
    [ -n "$checksum" ] || fail "could not determine the SHA-256 checksum for ${asset}"

    log "downloading ${asset}..."
    tf_curl -fsSL "${DOWNLOAD_URL}/${version_no_v}/${asset}" -o "${WORKDIR}/${asset}" \
        || fail "could not download ${asset}"
    verify_checksum "$checksum" "${WORKDIR}/${asset}"

    # The zip contains the 'terraform' binary at its root.
    unzip -q -o "${WORKDIR}/${asset}" -d "${WORKDIR}" \
        || fail "could not extract ${asset}"
    [ -f "${WORKDIR}/terraform" ] || fail "expected 'terraform' binary not found in ${asset}"
    chmod 0755 "${WORKDIR}/terraform"

    mkdir -p "$install_dir"
    mv "${WORKDIR}/terraform" "${install_dir}/terraform"
    "${install_dir}/terraform" version >/dev/null 2>&1 \
        || fail "installed terraform failed to run (${install_dir}/terraform)"
    log "installed Terraform ${version} to ${install_dir}/terraform"

    # Make terraform available to later GitHub Actions steps.
    if [ -n "${GITHUB_PATH:-}" ]; then
        echo "$install_dir" >> "$GITHUB_PATH"
    fi
}

trap cleanup EXIT
main "$@"
