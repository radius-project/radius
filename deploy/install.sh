#!/usr/bin/env bash

# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------.

# Radius CLI location
: ${RADIUS_INSTALL_DIR:="/usr/local/bin"}

# sudo is required to copy binary to RADIUS_INSTALL_DIR for linux and M1 macs
: ${USE_SUDO:="false"}

# Http request CLI
RADIUS_HTTP_REQUEST_CLI=curl

# Radius CLI filename
RADIUS_CLI_FILENAME=rad

RADIUS_CLI_FILE="${RADIUS_INSTALL_DIR}/${RADIUS_CLI_FILENAME}"

getSystemInfo() {
    ARCH=$(uname -m)
    case $ARCH in
        armv7*) ARCH="arm";;
        aarch64) ARCH="arm64";;
        x86_64) ARCH="x64";;
    esac

    OS=$(echo `uname`|tr '[:upper:]' '[:lower:]')

    if [ "$OS" == "darwin" ]; then
        OS="macos"
    fi

    # Most linux distro needs root permission to copy the file to /usr/local/bin
    # Also, for M1 macs, we also need sudo permission for /usr/local/bin
    if [[ ("$OS" == "linux" || ( "$OS" == "macos" && ( "$ARCH" == "arm" || "$ARCH" == "arm64" ))) && "$RADIUS_INSTALL_DIR" == "/usr/local/bin"  ]];
    then
        USE_SUDO="true"
    fi
}

verifySupported() {
    local supported=(macos-x64 macos-arm64 linux-x64 linux-arm linux-arm64)
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

runAsRoot() {
    local CMD="$*"

    if [ $EUID -ne 0 -a $USE_SUDO = "true" ]; then
        echo "Additional permissions needed. Please enter your sudo password..."
        CMD="sudo $CMD"
    fi

    $CMD
}

checkHttpRequestCLI() {
    if type "curl" > /dev/null; then
        RADIUS_HTTP_REQUEST_CLI=curl
    elif type "wget" > /dev/null; then
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
    local releaseUrl="https://get.radapp.dev/version/stable.txt"
    local latest_release=""

    if [ "$RADIUS_HTTP_REQUEST_CLI" == "curl" ]; then
        latest_release=$(curl -s $releaseUrl)
    else
        latest_release=$(wget -q -O - $releaseUrl)
    fi

    ret_val=$latest_release
}

downloadFile() {
    LATEST_RELEASE_TAG=$1

    OS_ARCH="${OS}-${ARCH}"
    RADIUS_CLI_ARTIFACT="rad"
    DOWNLOAD_BASE="https://get.radapp.dev/tools/rad"
    DOWNLOAD_URL="${DOWNLOAD_BASE}/${LATEST_RELEASE_TAG}/${OS_ARCH}/${RADIUS_CLI_ARTIFACT}"

    # Create the temp directory
    RADIUS_TMP_ROOT=$(mktemp -dt Radius-install-XXXXXX)
    ARTIFACT_TMP_FILE="$RADIUS_TMP_ROOT/$RADIUS_CLI_ARTIFACT"

    if [ "$RADIUS_HTTP_REQUEST_CLI" == "curl" ]; then
        if ! curl --output /dev/null --silent --head --fail "$DOWNLOAD_URL"; then
            echo "ERROR: The specified release version: $LATEST_RELEASE_TAG does not exist."
            exit 1
        fi
    else
        if ! wget --spider "$DOWNLOAD_URL" 2>/dev/null; then
            echo "ERROR: The specified release version: $LATEST_RELEASE_TAG does not exist."
            exit 1
        fi
    fi


    echo "Downloading ${DOWNLOAD_URL}..."
    if [ "$RADIUS_HTTP_REQUEST_CLI" == "curl" ]; then
        curl -SsL "$DOWNLOAD_URL" -o "$ARTIFACT_TMP_FILE"
    else
        wget -q -O "$ARTIFACT_TMP_FILE" "$DOWNLOAD_URL"
    fi

    if [ ! -f "$ARTIFACT_TMP_FILE" ]; then
        echo "failed to download ${DOWNLOAD_URL}..."
        exit 1
    fi
}

installFile() {
    local tmp_root_Radius_cli="$RADIUS_TMP_ROOT/$RADIUS_CLI_FILENAME"

    if [ ! -f "$tmp_root_Radius_cli" ]; then
        echo "Failed to download Radius CLI executable."
        exit 1
    fi

    chmod a+x $tmp_root_Radius_cli
    runAsRoot cp "$tmp_root_Radius_cli" "$RADIUS_INSTALL_DIR"

    if [ -f "$RADIUS_CLI_FILE" ]; then
        echo "$RADIUS_CLI_FILENAME installed into $RADIUS_INSTALL_DIR successfully."
        
        echo "Installing rad-bicep (\"rad bicep download\")..."
        $RADIUS_CLI_FILE bicep download
        result=$?
        if [ $result -eq 0 ]; then
            echo "rad-bicep installed successfully"
        else
           echo "Failed to install rad-bicep"
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
        echo "For support, go to https://github.com/project-radius/radius"
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
    echo -e "\nTo get started with Radius, please visit https://radapp.dev/getting-started/"
}

# -----------------------------------------------------------------------------
# main
# -----------------------------------------------------------------------------
trap "fail_trap" EXIT

getSystemInfo
verifySupported
checkExistingRadius
checkHttpRequestCLI


if [ -z "$1" ]; then
    echo "Getting the latest Radius CLI..."
    getLatestRelease
else
    ret_val=$1
    echo "Getting the Radius CLI release version: $1..."
fi

echo "Installing $ret_val Radius CLI..."

downloadFile $ret_val
installFile
cleanup

installCompleted
