---
type: docs
title: "Download and install the Radius CLI"
linkTitle: "Install Radius CLI"
description: "How to download and install the Radius CLI on your local machine"
weight: 10
---

These steps will setup the required tools and extensions to get you up and running with Radius.

## Pre-requisites

- [Az CLI](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli)

## Install CLI

{{< tabs Windows MacOS "Linux/WSL" "Cloud Shell" Binaries >}}

{{% codetab %}}

### PowerShell

```powershell
iwr -useb https://get.radapp.dev/tools/rad/install.ps1 | iex
```

{{% /codetab %}}
{{% codetab %}}

```bash
curl -fsSL "https://get.radapp.dev/tools/rad/install.sh" | /bin/bash
```

{{% /codetab %}}

{{% codetab %}}

```bash
wget -q "https://get.radapp.dev/tools/rad/install.sh" -O - | /bin/bash
```

{{% /codetab %}}

{{% codetab %}}

[Azure Cloud Shell](https://docs.microsoft.com/en-us/azure/cloud-shell/overview) is an interactive, authenticated, browser-accessible shell for managing Azure resources.

### Bash

Azure Cloud Shell for bash doesn't have a sudo command, so users are unable to install Radius to the default `/usr/local/bin` installation path. To install the rad CLI to the home directory, run the following commands:

```bash
export RADIUS_INSTALL_DIR=./
wget -q "https://get.radapp.dev/tools/rad/install.sh" -O - | /bin/bash
```

You can now run the rad CLI with `./rad`.

### PowerShell

PowerShell for Cloud Shell is currently not supported.

{{% /codetab %}}

{{% codetab %}}

1. Download the `rad` CLI from one of these URLs:

   - MacOS: https://get.radapp.dev/tools/rad/0.5/macos-x64/rad
   - Linux: https://get.radapp.dev/tools/rad/0.5/linux-x64/rad
   - Windows: https://get.radapp.dev/tools/rad/0.5/windows-x64/rad.exe

1. Ensure the user has permission to execute the binary and place it somewhere on your PATH so it can be invoked easily.

{{% /codetab %}}

{{< /tabs >}}

For unstable (edge) versions, visit the [edge docs](https://edge.radapp.dev/getting-started/install-cli/).

## Test it out

1. Verify the rad CLI is installed correctly:

   ```bash
   $ rad
   
   Usage:
     rad [command]
   
   Available Commands:
     application Manage applications
     bicep       Manage bicep compiler
     component   Manage components
     deploy      Deploy a RAD application
     deployment  Manage deployments
     env         Manage environments
     expose      Expose local port
     help        Help about any command
   
   Flags:
         --config string   config file (default is $HOME/.rad/config.yaml)
     -h, --help            help for rad
     -v, --version         version for rad
   
   Use "rad [command] --help" for more information about a command.
   ```

{{< button text="Next: Setup VSCode" page="setup-vscode.md" >}}
