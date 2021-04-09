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

{{< tabs Windows MacOS Linux Binaries >}}

{{% codetab %}}

### Install the latest stable version

```
powershell -Command "iwr -useb https://raw.githubusercontent.com/azure/radius/main/install/install.ps1 | iex"
```

### Install a specific version

```
powershell -Command "$script=iwr -useb  https://raw.githubusercontent.com/azure/radius/main/install/install.ps1; $block=[ScriptBlock]::Create($script); invoke-command -ScriptBlock $block -ArgumentList <Version>"
```

{{% /codetab %}}
{{% codetab %}}

### Install the latest stable version

```
curl -fsSL "https://raw.githubusercontent.com/azure/radius/main/install/install.sh" | /bin/bash
```

### Install a specific version

```
curl -fsSL "https://raw.githubusercontent.com/azure/radius/main/install/install.sh" | /bin/bash -s <Version>
```

{{% /codetab %}}

{{% codetab %}}

### Install the latest stable version

```
wget -q "https://raw.githubusercontent.com/azure/radius/main/install/install.sh" | /bin/bash
```

### Install a specific version

```
wget -q "https://raw.githubusercontent.com/azure/radius/main/install/install.sh" | /bin/bash -s <Version>
```

{{% codetab %}}

{{% codetab %}}

### Install the latest stable version

1. Download the `rad` CLI from one of these URLs:

   - MacOS: https://radiuspublic.blob.core.windows.net/tools/rad/edge/macos-x64/rad
   - Linux: https://radiuspublic.blob.core.windows.net/tools/rad/edge/linux-x64/rad
   - Windows: https://radiuspublic.blob.core.windows.net/tools/rad/edge/windows-x64/rad.exe

1. Ensure the user has permission to execute the binary and place it somewhere on your PATH so it can be invoked easily.

### Install a specific version

1. Download the `rad` CLI from one of these URLs (replace `<version>` with your desired version):

   - MacOS: https://radiuspublic.blob.core.windows.net/tools/rad/<version>/macos-x64/rad
   - Linux: https://radiuspublic.blob.core.windows.net/tools/rad/edge/linux-x64/rad
   - Windows: https://radiuspublic.blob.core.windows.net/tools/rad/edge/windows-x64/rad.exe

2. Ensure the user has permission to execute the binary and place it somewhere on your PATH so it can be invoked easily.


## Test it out

1. Verify the rad CLI is installed correctly:

   ```bash
   $ rad
   
   Usage:
     rad [command]
   
   Available Commands:
     application Manage applications
     bicep       Manage bicep compiler
     deploy      Deploy a RAD application
     env         Manage environments
     expose      Expose local port
     help        Help about any command
   
   Flags:
         --config string   config file (default is $HOME/.rad/config.yaml)
     -h, --help            help for rad
   
   Use "rad [command] --help" for more information about a command.
   ```

<br /><a class="btn btn-primary" href="{{< ref setup-vscode.md >}}" role="button">Next: Setup VSCode</a>
