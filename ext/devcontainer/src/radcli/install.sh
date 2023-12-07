#!/usr/bin/env bash
set -e

CLI_VERSION="${VERSION:-"latest"}" 

# Ensure apt is in non-interactive to avoid prompts
export DEBIAN_FRONTEND=noninteractive

check_packages() {
    if ! dpkg -s "$@" > /dev/null 2>&1; then
        if [ "$(find /var/lib/apt/lists/* | wc -l)" = "0" ]; then
            echo "Running apt-get update..."
            apt-get update -y
        fi
        apt-get -y install --no-install-recommends "$@"
    fi
}

echo "(*) Ensuring dependencies are installed"

check_packages wget

echo "(*) Installing Radius CLI"

# If version is not specified, install latest otherwise install edge
if [ "${CLI_VERSION}" = "latest" ]; then
    wget -q "https://raw.githubusercontent.com/radius-project/radius/main/deploy/install.sh" -O - | /bin/bash
else
    wget -O /usr/local/bin/rad https://get.radapp.dev/tools/rad/edge/linux-x64/rad 
    chmod +rx /usr/local/bin/rad
fi
