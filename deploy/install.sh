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

# Radius CLI location
: ${RADIUS_INSTALL_DIR:="/usr/local/bin"}

# sudo is required to copy binary to RADIUS_INSTALL_DIR for linux
: ${USE_SUDO:="false"}

# Http request CLI
RADIUS_HTTP_REQUEST_CLI=curl

# GitHub Organization and repo name to download release
GITHUB_ORG=radius-project
GITHUB_REPO=radius

# Radius CLI filename
RADIUS_CLI_FILENAME=rad

RADIUS_CLI_FILE="${RADIUS_INSTALL_DIR}/${RADIUS_CLI_FILENAME}"

getSystemInfo() {
    ARCH=$(uname -m)
    case $ARCH in
        armv7*) ARCH="arm";;
        aarch64) ARCH="arm64";;
        x86_64) ARCH="amd64";;
    esac

    OS=$(echo `uname`|tr '[:upper:]' '[:lower:]')

    # Most linux distro needs root permission to copy the file to /usr/local/bin
    if [[ "$OS" == "linux" || "$OS" == "darwin" ]] && [ "$RADIUS_INSTALL_DIR" == "/usr/local/bin" ]; then
        USE_SUDO="true"
    fi
}

verifySupported() {
    local supported=(darwin-arm64 darwin-amd64 linux-amd64 linux-arm linux-arm64)
    local current_osarch="${OS}-${ARCH}"

    for osarch in "${supported[@]}"; do
        if [ "$osarch" == "$current_osarch" ]; then
            echo "Your system is ${OS}_${ARCH}"
            return
        fi
    done

    echo "No prebuilt binary for ${current_osarch}"
    exit 1
}

getManifestToBicepExtensionBinaryName() {
    local platform="${OS}-${ARCH}"
    
    case $platform in
        "darwin-amd64")
            echo "manifest-to-bicep-extension-darwin-amd64"
            ;;
        "darwin-arm64")
            echo "manifest-to-bicep-extension-darwin-arm64"
            ;;
        "linux-amd64")
            echo "manifest-to-bicep-extension-linux-amd64"
            ;;
        "linux-arm64")
            echo "manifest-to-bicep-extension-linux-arm64"
            ;;
        "linux-arm")
            # bicep-tools doesn't provide linux-arm, skip
            echo ""
            ;;
        *)
            echo ""
            ;;
    esac
}

downloadManifestToBicepExtension() {
    local binary_name=$(getManifestToBicepExtensionBinaryName)
    
    if [ -z "$binary_name" ]; then
        echo "manifest-to-bicep-extension is not available for ${OS}-${ARCH}, skipping..."
        return 0
    fi
    
    local home_dir
    home_dir=$(eval echo ~$USER)
    local install_dir="${home_dir}/.rad/bin"
    local binary_path="${install_dir}/manifest-to-bicep-extension"
    
    # Create the install directory if it doesn't exist
    mkdir -p "$install_dir"
    
    local download_url="https://github.com/willdavsmith/bicep-tools/releases/download/v0.2.0/${binary_name}"
    
    echo "Downloading manifest-to-bicep-extension from ${download_url}..."
    
    if [ "$RADIUS_HTTP_REQUEST_CLI" == "curl" ]; then
        curl -SsL "$download_url" -o "$binary_path"
    else
        wget -q -O "$binary_path" "$download_url"
    fi
    
    if [ ! -f "$binary_path" ]; then
        echo "Failed to download manifest-to-bicep-extension"
        return 1
    fi
    
    # Make the binary executable
    chmod +x "$binary_path"
    
    echo "manifest-to-bicep-extension installed successfully"
    return 0
}

runAsRoot() {
    local CMD="$*"

    if [ $EUID -ne 0 -a $USE_SUDO = "true" ]; then
        CMD="sudo $CMD"
    fi

    $CMD
}

checkHttpRequestCLI() {
    if type "curl" &> /dev/null; then
        RADIUS_HTTP_REQUEST_CLI=curl
    elif type "wget" &> /dev/null; then
        RADIUS_HTTP_REQUEST_CLI=wget
    else
        echo "Either curl or wget is required"
        exit 1
    fi
}

checkExistingRadius() {
    if [ -f "$RADIUS_CLI_FILE" ]; then
        version=$($RADIUS_CLI_FILE version --cli)
        echo -e "\nRadius CLI is detected. Current version: ${version}"
        echo -e "Reinstalling Radius CLI - ${RADIUS_CLI_FILE}...\n"
    else
        echo -e "Installing Radius CLI...\n"
    fi
}

getLatestRelease() {
    local radReleaseUrl="https://api.github.com/repos/${GITHUB_ORG}/${GITHUB_REPO}/releases"
    local latest_release=""

    if [ "$RADIUS_HTTP_REQUEST_CLI" == "curl" ]; then
        latest_release=$(curl -s $radReleaseUrl | grep \"tag_name\" | grep -v rc | awk 'NR==1{print $2}' |  sed -n 's/\"\(.*\)\",/\1/p')
    else
        latest_release=$(wget -q --header="Accept: application/json" -O - $radReleaseUrl | grep \"tag_name\" | grep -v rc | awk 'NR==1{print $2}' |  sed -n 's/\"\(.*\)\",/\1/p')
    fi

    ret_val=$latest_release
}

downloadFile() {
    RELEASE_TAG=$1

    RADIUS_CLI_ARTIFACT="${RADIUS_CLI_FILENAME}_${OS}_${ARCH}"

    # Create the temp directory
    RADIUS_TMP_ROOT=$(mktemp -dt radius-install-XXXXXX)
    ARTIFACT_TMP_FILE="$RADIUS_TMP_ROOT/$RADIUS_CLI_ARTIFACT"

    if [ "$RELEASE_TAG" == "edge" ]; then
        if ! command -v oras &> /dev/null; then
            echo "Error: oras CLI is not installed or not found in PATH."
            echo "Please visit https://edge.docs.radapp.io/installation for edge CLI installation instructions."
            exit 1
        fi

        DOWNLOAD_URL="ghcr.io/radius-project/rad/${OS}-${ARCH}:latest"
        echo "Downloading edge CLI from ${DOWNLOAD_URL}..."
        oras pull $DOWNLOAD_URL -o $RADIUS_TMP_ROOT

        # Check if the oras pull command was successfull
        if [ $? -ne 0 ]; then
            echo "Failed to download edge CLI."
            echo "If this was an authentication issue, please run 'docker logout ghcr.io' to clear any expired credentials."
            echo "Visit https://edge.docs.radapp.io/installation for edge CLI installation instructions."
            exit 1
        fi

        mv $RADIUS_TMP_ROOT/rad $ARTIFACT_TMP_FILE
    else
        DOWNLOAD_BASE="https://github.com/${GITHUB_ORG}/${GITHUB_REPO}/releases/download"
        DOWNLOAD_URL="${DOWNLOAD_BASE}/${RELEASE_TAG}/${RADIUS_CLI_ARTIFACT}"

        echo "Downloading ${DOWNLOAD_URL}..."
        if [ "$RADIUS_HTTP_REQUEST_CLI" == "curl" ]; then
            curl -SsL "$DOWNLOAD_URL" -o "$ARTIFACT_TMP_FILE"
        else
            wget -q -O "$ARTIFACT_TMP_FILE" "$DOWNLOAD_URL"
        fi
    fi

    if [ ! -f "$ARTIFACT_TMP_FILE" ]; then
        echo "failed to download ${DOWNLOAD_URL}..."
        exit 1
    fi
}

installFile() {
    RADIUS_CLI_ARTIFACT="${RADIUS_CLI_FILENAME}_${OS}_${ARCH}"
    local tmp_root_radius_cli="$RADIUS_TMP_ROOT/$RADIUS_CLI_ARTIFACT"

    if [ ! -f "$tmp_root_radius_cli" ]; then
        echo "Failed to unpack Radius CLI executable."
        exit 1
    fi

    if [ -f "$RADIUS_CLI_FILE" ]; then
        runAsRoot rm "$RADIUS_CLI_FILE"
    fi
    chmod a+x $tmp_root_radius_cli
    mkdir -p $RADIUS_INSTALL_DIR
    runAsRoot cp "$tmp_root_radius_cli" "$RADIUS_INSTALL_DIR"
    runAsRoot mv "${RADIUS_INSTALL_DIR}/${RADIUS_CLI_ARTIFACT}" "${RADIUS_INSTALL_DIR}/${RADIUS_CLI_FILENAME}"

    if [ -f "$RADIUS_CLI_FILE" ]; then
        echo "$RADIUS_CLI_FILENAME installed into $RADIUS_INSTALL_DIR successfully"

        echo "Installing rad-bicep (\"rad bicep download\")..."
        $RADIUS_CLI_FILE bicep download
        result=$?
        if [ $result -eq 0 ]; then
            echo "rad-bicep installed successfully"
        else
           echo "Failed to install rad-bicep"
           exit 1
        fi

        echo "Installing manifest-to-bicep-extension..."
        downloadManifestToBicepExtension
        result=$?
        if [ $result -eq 0 ]; then
            echo "manifest-to-bicep-extension installation completed"
        else
           echo "Failed to install manifest-to-bicep-extension"
           exit 1
        fi

        # TODO: $RADIUS_CLI_FILE --version
    else 
        echo "Failed to install $RADIUS_CLI_FILENAME"
        exit 1
    fi
}

fail_trap() {
    result=$?
    if [ "$result" != "0" ]; then
        echo "Failed to install Radius CLI"
        echo "For support, go to https://github.com/radius-project/radius"
    fi
    cleanup
    exit $result
}

cleanup() {
    if [[ -d "${RADIUS_TMP_ROOT:-}" ]]; then
        rm -rf "$RADIUS_TMP_ROOT"
    fi
}

installCompleted() {
    echo -e "\nTo get started with Radius, please visit https://docs.radapp.io/getting-started/"
}

# -----------------------------------------------------------------------------
# main
# -----------------------------------------------------------------------------
trap "fail_trap" EXIT

getSystemInfo
checkHttpRequestCLI

if [ -z "$1" ]; then
    echo "Getting the latest Radius CLI..."
    getLatestRelease
elif [ "$1" == "edge" ]; then
    ret_val="edge"
else
    ret_val=v$1
fi

verifySupported "$ret_val"
checkExistingRadius

echo "Installing $ret_val Radius CLI..."

downloadFile "$ret_val"
installFile
cleanup

installCompleted