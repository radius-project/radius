#!/usr/bin/env bash

# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

# ============================================================================
# Radius CLI Installer
#
# Usage:
#   wget -qO- https://raw.githubusercontent.com/radius-project/radius/main/deploy/install.sh | /bin/bash
#   wget -qO- ... | /bin/bash -s -- --version 0.40.0
#   wget -qO- ... | /bin/bash -s -- --install-dir ~/.local/bin
#   wget -qO- ... | /bin/bash -s -- --version 0.40.0 --install-dir /opt/bin
#   wget -qO- ... | /bin/bash -s -- --include-rc
#   wget -qO- ... | sudo /bin/bash -s -- --install-dir /usr/local/bin
#
# Environment variables (override flags):
#   INSTALL_DIR         - Installation directory (default: auto-detected)
#   INCLUDE_RC          - Include release candidates ("true"/"false")
# ============================================================================

set -euo pipefail

# Include release candidates when determining latest version
: "${INCLUDE_RC:="false"}"

# Http request CLI
RADIUS_HTTP_REQUEST_CLI="curl"

# GitHub Organization and repo name to download release
readonly GITHUB_ORG="radius-project"
readonly GITHUB_REPO="radius"

# Radius CLI filename
readonly RADIUS_CLI_FILENAME="rad"

# Temp directory for downloads (set in downloadFile, cleaned up on exit)
RADIUS_TMP_ROOT=""

# OS and ARCH are set in getSystemInfo
OS=""
ARCH=""

usage() {
    cat << EOF
Radius CLI Installer

Usage: install.sh [OPTIONS] [VERSION]

Options:
  -d, --install-dir <DIR>  Installation directory (default: auto-detected)
  -v, --version <VERSION>  Version to install (e.g., 0.40.0, edge)
  -rc, --include-rc        Include release candidates in latest version
  -h, --help               Show this help message

Environment Variables:
  INSTALL_DIR              Override installation directory
  INCLUDE_RC               Set to "true" to include release candidates

Install Directory Detection:
  - If run as root or with sudo available: /usr/local/bin
  - Otherwise: \$HOME/.local/bin

Examples:
  # Install latest stable version
  ./install.sh

  # Install a specific version
  ./install.sh --version 0.40.0

  # Install to a custom directory (no root required)
  ./install.sh --install-dir ~/.local/bin

  # Install the edge (development) version
  ./install.sh --version edge

EOF
    exit 0
}

getSystemInfo() {
    ARCH=$(uname -m)
    case "${ARCH}" in
        armv7*) ARCH="arm" ;;
        aarch64) ARCH="arm64" ;;
        x86_64) ARCH="amd64" ;;
        *) ;; # Other architectures are validated in verifySupported
    esac

    OS=$(uname | tr '[:upper:]' '[:lower:]')
}

# Determine the default install directory based on permissions.
# Follows the pattern used by fnm/uv: use /usr/local/bin when root,
# otherwise fall back to $HOME/.local/bin (always writable).
getDefaultInstallDir() {
    if [[ -n "${INSTALL_DIR:-}" ]]; then
        # User explicitly set via environment variable or flag
        echo "${INSTALL_DIR}"
        return
    fi

    if [[ ${EUID:-$(id -u)} -eq 0 ]]; then
        echo "/usr/local/bin"
        return
    fi

    # Non-root: use a user-writable location
    echo "${HOME}/.local/bin"
}

# Determine whether sudo is needed and available for the chosen install dir.
needsSudo() {
    local install_dir="$1"

    # Root never needs sudo
    if [[ ${EUID:-$(id -u)} -eq 0 ]]; then
        return 1
    fi

    # If the directory exists and is writable, no sudo needed
    if [[ -d "${install_dir}" && -w "${install_dir}" ]]; then
        return 1
    fi

    # If the parent directory is writable (dir doesn't exist yet), no sudo
    local parent_dir
    parent_dir=$(dirname "${install_dir}")
    if [[ -d "${parent_dir}" && -w "${parent_dir}" ]]; then
        return 1
    fi

    # Need elevated privileges
    return 0
}

runAsRoot() {
    if needsSudo "${INSTALL_DIR}"; then
        if command -v sudo &> /dev/null; then
            sudo "$@"
        else
            echo "Error: installation to ${INSTALL_DIR} requires root privileges."
            echo "Either run as root, install sudo, or use --install-dir to choose a user-writable location."
            echo "  Example: $0 --install-dir \"\${HOME}/.local/bin\""
            exit 1
        fi
    else
        "$@"
    fi
}

verifySupported() {
    local supported=(darwin-arm64 darwin-amd64 linux-amd64 linux-arm linux-arm64)
    local current_osarch="${OS}-${ARCH}"

    for osarch in "${supported[@]}"; do
        if [[ "${osarch}" == "${current_osarch}" ]]; then
            echo "Your system is ${OS}_${ARCH}"
            return
        fi
    done

    echo "No prebuilt binary for ${current_osarch}"
    exit 1
}

checkHttpRequestCLI() {
    if command -v curl &> /dev/null; then
        RADIUS_HTTP_REQUEST_CLI=curl
    elif command -v wget &> /dev/null; then
        RADIUS_HTTP_REQUEST_CLI=wget
    else
        echo "Either curl or wget is required"
        exit 1
    fi
}

checkExistingRadius() {
    local cli_file="${INSTALL_DIR}/${RADIUS_CLI_FILENAME}"
    if [[ -f "${cli_file}" ]]; then
        local version
        version=$("${cli_file}" version --cli 2> /dev/null || echo "unknown")
        printf '\nRadius CLI is detected. Current version: %s\n\n' "${version}"
        printf 'Reinstalling Radius CLI - %s...\n\n' "${cli_file}"
    else
        printf 'Installing Radius CLI...\n\n'
    fi
}

# Warn if existing rad binaries are found in PATH at different locations.
warnExistingRadiusElsewhere() {
    # Resolve the target install directory to an absolute path without creating it
    local resolved_install
    if [[ -d "${INSTALL_DIR}" ]]; then
        if ! resolved_install=$(cd "${INSTALL_DIR}" && pwd -P); then
            resolved_install="${INSTALL_DIR}"
        fi
    else
        resolved_install="${INSTALL_DIR}"
    fi

    # Walk every PATH directory looking for rad binaries elsewhere
    local stale_paths=()
    local IFS=':'
    for dir in ${PATH}; do
        local path_dir="${dir}"
        # shellcheck disable=SC2088
        # Match literal '~' and '~/' PATH entries so they can be normalized to
        # $HOME before checking for alternate rad binaries.
        if [[ "${path_dir}" == "~" ]]; then
            path_dir="${HOME}"
        elif [[ "${path_dir}" == "~/"* ]]; then
            path_dir="${HOME}/${path_dir:2}"
        fi

        local candidate="${path_dir}/${RADIUS_CLI_FILENAME}"
        if [[ -x "${candidate}" ]]; then
            local resolved_dir
            resolved_dir=$(cd "${path_dir}" 2> /dev/null && pwd -P) || continue
            if [[ "${resolved_dir}" != "${resolved_install}" ]]; then
                stale_paths+=("${candidate}")
            fi
        fi
    done

    if (( ${#stale_paths[@]} == 0 )); then
        return
    fi

    echo "============================================================================"
    echo "WARNING: Existing Radius CLI installation(s) found in different location(s):"
    for p in "${stale_paths[@]}"; do
        echo "  ${p}"
    done
    echo ""
    echo "The new installation will be placed in:"
    echo "  ${INSTALL_DIR}/${RADIUS_CLI_FILENAME}"
    echo ""
    echo "Remove the old binary(ies) before continuing to avoid using the wrong version:"
    for p in "${stale_paths[@]}"; do
        echo "  rm -- \"${p}\""
    done
    echo "============================================================================"
}

getLatestRelease() {
    local radReleaseUrl="https://api.github.com/repos/${GITHUB_ORG}/${GITHUB_REPO}/releases"
    local latest_release=""

    if [[ "${INCLUDE_RC}" == "true" ]]; then
        if [[ "${RADIUS_HTTP_REQUEST_CLI}" == "curl" ]]; then
            latest_release=$(curl -s "${radReleaseUrl}" | grep \"tag_name\" | awk 'NR==1{print $2}' | sed -n 's/\"\(.*\)\",/\1/p')
        else
            latest_release=$(wget -q --header="Accept: application/json" -O - "${radReleaseUrl}" | grep \"tag_name\" | awk 'NR==1{print $2}' | sed -n 's/\"\(.*\)\",/\1/p')
        fi
    else
        if [[ "${RADIUS_HTTP_REQUEST_CLI}" == "curl" ]]; then
            latest_release=$(curl -s "${radReleaseUrl}" | grep \"tag_name\" | grep -v rc | awk 'NR==1{print $2}' | sed -n 's/\"\(.*\)\",/\1/p')
        else
            latest_release=$(wget -q --header="Accept: application/json" -O - "${radReleaseUrl}" | grep \"tag_name\" | grep -v rc | awk 'NR==1{print $2}' | sed -n 's/\"\(.*\)\",/\1/p')
        fi
    fi

    if [[ -z "${latest_release}" ]]; then
        echo "Error: could not determine latest release"
        exit 1
    fi

    ret_val="${latest_release}"
}

downloadFile() {
    local release_tag="$1"

    local radius_cli_artifact="${RADIUS_CLI_FILENAME}_${OS}_${ARCH}"

    RADIUS_TMP_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/radius-install-XXXXXX")
    local artifact_tmp_file="${RADIUS_TMP_ROOT}/${radius_cli_artifact}"

    if [[ "${release_tag}" == "edge" ]]; then
        if ! command -v oras &> /dev/null; then
            echo "Error: oras CLI is not installed or not found in PATH."
            echo "Please visit https://edge.docs.radapp.io/installation for edge CLI installation instructions."
            exit 1
        fi

        local download_url="ghcr.io/radius-project/rad/${OS}-${ARCH}:latest"
        echo "Downloading edge CLI from ${download_url}..."
        if ! oras pull "${download_url}" -o "${RADIUS_TMP_ROOT}"; then
            echo "Failed to download edge CLI."
            echo "If this was an authentication issue, please run 'docker logout ghcr.io' to clear any expired credentials."
            echo "Visit https://edge.docs.radapp.io/installation for edge CLI installation instructions."
            exit 1
        fi

        mv "${RADIUS_TMP_ROOT}/rad" "${artifact_tmp_file}"
    else
        local download_base="https://github.com/${GITHUB_ORG}/${GITHUB_REPO}/releases/download"
        local download_url="${download_base}/${release_tag}/${radius_cli_artifact}"

        echo "Downloading ${download_url}..."
        if [[ "${RADIUS_HTTP_REQUEST_CLI}" == "curl" ]]; then
            curl -SsL "${download_url}" -o "${artifact_tmp_file}"
        else
            wget -q -O "${artifact_tmp_file}" "${download_url}"
        fi
    fi

    if [[ ! -f "${artifact_tmp_file}" ]]; then
        echo "Failed to download ${download_url:-the artifact}..."
        exit 1
    fi
}

installFile() {
    local radius_cli_artifact="${RADIUS_CLI_FILENAME}_${OS}_${ARCH}"
    local tmp_root_radius_cli="${RADIUS_TMP_ROOT}/${radius_cli_artifact}"
    local cli_file="${INSTALL_DIR}/${RADIUS_CLI_FILENAME}"

    if [[ ! -f "${tmp_root_radius_cli}" ]]; then
        echo "Failed to unpack Radius CLI executable."
        exit 1
    fi

    if [[ -f "${cli_file}" ]]; then
        runAsRoot rm "${cli_file}"
    fi

    chmod a+x "${tmp_root_radius_cli}"
    runAsRoot mkdir -p "${INSTALL_DIR}"
    runAsRoot cp "${tmp_root_radius_cli}" "${cli_file}"

    if [[ -f "${cli_file}" ]]; then
        echo "${RADIUS_CLI_FILENAME} installed into ${INSTALL_DIR} successfully"

        echo "Installing bicep (\"rad bicep download\")..."
        if "${cli_file}" bicep download; then
            echo "bicep installed successfully"
        else
            echo "Failed to install bicep"
            exit 1
        fi
    else
        echo "Failed to install ${RADIUS_CLI_FILENAME}"
        exit 1
    fi
}

failTrap() {
    local result=$?
    if [[ "${result}" != "0" ]]; then
        echo "Failed to install Radius CLI"
        echo "For support, go to https://github.com/radius-project/radius"
    fi
    cleanup
    exit "${result}"
}

cleanup() {
    if [[ -d "${RADIUS_TMP_ROOT:-}" ]]; then
        rm -rf "${RADIUS_TMP_ROOT}"
    fi
}

# Print post-install guidance, including PATH hints when needed.
installCompleted() {
    echo ""

    # Check if the install dir is in PATH
    if ! echo "${PATH}" | tr ':' '\n' | grep -Fqx "${INSTALL_DIR}"; then
        echo "============================================================================"
        echo "NOTE: ${INSTALL_DIR} is not in your \$PATH."
        echo ""
        echo "Add it by running one of the following:"
        echo ""

        local current_shell
        current_shell="$(basename "${SHELL:-bash}")"
        case "${current_shell}" in
            zsh)
                echo "  echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.zshrc"
                echo "  source ~/.zshrc"
                ;;
            fish)
                echo "  fish_add_path ${INSTALL_DIR}"
                ;;
            *)
                echo "  echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.bashrc"
                echo "  source ~/.bashrc"
                ;;
        esac
        echo "============================================================================"
    fi

    echo "To get started with Radius, please visit https://docs.radapp.io/getting-started/"
}

# -----------------------------------------------------------------------------
# main
# -----------------------------------------------------------------------------
trap "failTrap" EXIT

getSystemInfo
checkHttpRequestCLI

# Parse command-line arguments
VERSION_ARG=""
while [[ $# -gt 0 ]]; do
    case $1 in
        -v | --version)
            VERSION_ARG="$2"
            shift 2
            ;;
        -d | --install-dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        -rc | --include-rc)
            INCLUDE_RC="true"
            shift
            ;;
        -h | --help)
            usage
            ;;
        -*)
            echo "Unknown option: $1"
            echo "Run '$0 --help' for usage."
            exit 1
            ;;
        *)
            # Support legacy positional version argument for backward compatibility
            VERSION_ARG="$1"
            shift
            ;;
    esac
done

# Support RADIUS_INSTALL_DIR as a backward-compatible alias for INSTALL_DIR
if [[ -n "${RADIUS_INSTALL_DIR:-}" && -z "${INSTALL_DIR:-}" ]]; then
    INSTALL_DIR="${RADIUS_INSTALL_DIR}"
fi

# Set install directory (after arg parsing so --install-dir takes effect)
INSTALL_DIR=$(getDefaultInstallDir)

if [[ -z "${VERSION_ARG}" ]]; then
    echo "Getting the latest Radius CLI..."
    if [[ "${INCLUDE_RC}" == "true" ]]; then
        echo "Including release candidates in version selection..."
    fi
    getLatestRelease
elif [[ "${VERSION_ARG}" == "edge" ]]; then
    ret_val="edge"
else
    # Strip leading "v" if present, then normalize to v-prefixed format
    ret_val="v${VERSION_ARG#v}"
fi

verifySupported
checkExistingRadius
warnExistingRadiusElsewhere

echo "Installing ${ret_val} Radius CLI..."
echo "Install directory: ${INSTALL_DIR}"

downloadFile "${ret_val}"
installFile
cleanup

installCompleted
