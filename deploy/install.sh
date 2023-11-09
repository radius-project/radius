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
    LATEST_RELEASE_TAG=$1

    RADIUS_CLI_ARTIFACT="${RADIUS_CLI_FILENAME}_${OS}_${ARCH}"
    DOWNLOAD_BASE="https://github.com/${GITHUB_ORG}/${GITHUB_REPO}/releases/download"
    DOWNLOAD_URL="${DOWNLOAD_BASE}/${LATEST_RELEASE_TAG}/${RADIUS_CLI_ARTIFACT}"

    # Create the temp directory
    RADIUS_TMP_ROOT=$(mktemp -dt radius-install-XXXXXX)
    ARTIFACT_TMP_FILE="$RADIUS_TMP_ROOT/$RADIUS_CLI_ARTIFACT"

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

isReleaseAvailable() {
    LATEST_RELEASE_TAG=$1

    RADIUS_CLI_ARTIFACT="${RADIUS_CLI_FILENAME}_${OS}_${ARCH}"
    DOWNLOAD_BASE="https://github.com/${GITHUB_ORG}/${GITHUB_REPO}/releases/download"
    DOWNLOAD_URL="${DOWNLOAD_BASE}/${LATEST_RELEASE_TAG}/${RADIUS_CLI_ARTIFACT}"

    if [ "$RADIUS_HTTP_REQUEST_CLI" == "curl" ]; then
        httpstatus=$(curl -sSLI -o /dev/null -w "%{http_code}" "$DOWNLOAD_URL")
        if [ "$httpstatus" == "200" ]; then
            return 0
        fi
    else
        wget -q --spider "$DOWNLOAD_URL"
        exitstatus=$?
        if [ $exitstatus -eq 0 ]; then
            return 0
        fi
    fi
    return 1
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
else
    ret_val=v$1
fi

verifySupported $ret_val
checkExistingRadius

echo "Installing $ret_val Radius CLI..."

downloadFile $ret_val
installFile
cleanup

installCompleted