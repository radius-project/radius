---
type: docs
title: "Radius CLI"
linkTitle: "CLI"
description: "Learn how to manage Radius environments and applications using the rad CLI"
weight: 100
---

The rad CLI makes it easy to initialize, deploy, manage, and monitor Radius environments and applications.

## Features

### Environments management

Use the following comments to interact with environments:

{{< tabs Initialize List Show Switch Delete >}}

{{% codetab %}}
Use the [`rad env init` command]({{< ref rad_env_init >}}) to initialize an environments on an [Azure]({{< ref azure >}}), [Kubernetes]({{< ref kubernetes >}}) or [local dev]({{< ref local >}}) platform.

```sh
rad env init azure -i
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad env list` command]({{< ref rad_env_list >}}) to list all environments that you have initialized.

```sh
rad env list
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad env show` command]({{< ref rad_env_show >}}) to show the details of an environment.

```sh
rad env show -e my-env
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad env switch` command]({{< ref rad_env_switch >}}) to switch your default environment.

```sh
rad env switch -e my-env
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad env delete` command]({{< ref rad_env_delete >}}) to delete an environment.

```sh
rad env delete
```

{{% /codetab %}}

{{% /tabs %}}

### Application management

Use the following comments to interact with applications:

{{< tabs Initialize List Show Switch Run Deploy Delete >}}

{{% codetab %}}
Use the [`rad application init` command]({{< ref rad_application_init >}}) to initialize a new Radius application in your current directory.

```sh
rad application init -a my-app
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad application list` command]({{< ref rad_application_list >}}) to list all applications deployed to an environment.

```sh
rad application list
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad application show` command]({{< ref rad_application_show >}}) to show the details of an application.

```sh
rad application show -a my-app
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad application switch` command]({{< ref rad_application_switch >}}) to switch your default application.

```sh
rad application switch -a my-app
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad application run` command]({{< ref rad_application_run >}}) to run an application in a local dev environment.

```sh
rad application run
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad application deploy` command]({{< ref rad_application_show >}}) to deploy an application to an environment.

```sh
rad application deploy
```

Visit the [multi-stage deployments docs]({{< ref multi-stage-deployments >}}) for more information.

{{% /codetab %}}
{{% codetab %}}
Use the [`rad application delete` command]({{< ref rad_application_delete >}}) to delete an application from an environment.

```sh
rad application delete -a my-app
```

{{% /codetab %}}

{{% /tabs %}}

### Resource management

Use the following comments to interact with application resources:

{{< tabs List Show Logs Expose >}}

{{% codetab %}}
Use the [`rad resource list` command]({{< ref rad_resource_list >}}) to list all deployed resources in an application.

```sh
rad resource list -a my-app
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad resource show` command]({{< ref rad_resource_show >}}) to show the details of an application resource.

```sh
rad resource show -r frontend -a my-app
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad resource logs` command]({{< ref rad_resource_logs >}}) to view the logs of a service.

```sh
rad resource logs -r frontend -a my-app
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad resource expose` command]({{< ref rad_resource_list >}}) to port-forward a service to your local machine. Only compatible with [containers]({{< ref container >}}).

```sh
rad resource list -a my-app
```

{{% /codetab %}}

{{% /tabs %}}

### Template deployment

Use the [`rad deploy` command]({{< ref rad_deploy >}}) to deploy an entire Bicep template to an environment. For additional features such as multi-stage deployments and deployment profiles, it is recommended to use the [application deployment](#application-management) instead

```sh
rad deploy ./template.bicep
```

### Terminal completion

Use the [`rad completion` commands]({{< ref rad_completion >}}) to generate shell completion scripts for your terminal, allowing you to tab-complete commands.

```powershell
rad completion powershell >> $PROFILE
```

### Bicep management

Use the following comments to interact with the Bicep compiler:

{{< tabs Download Delete >}}

{{% codetab %}}
Use the [`rad bicep download` command]({{< ref rad_bicep_download >}}) to download the latest Bicep compiler.

```sh
rad bicep download
```

{{% /codetab %}}

{{% codetab %}}
Use the [`rad bicep delete` command]({{< ref rad_bicep_delete >}}) to delete the Bicep compiler currently downloaded.

```sh
rad bicep delete
```

{{% /codetab %}}

{{% /tabs %}}

## Install the rad CLI

The rad CLI is available on Windows, macOS, and Linux:

{{< tabs Windows MacOS "Linux/WSL" "Cloud Shell" Binaries >}}

{{% codetab %}}

Run the following within PowerShell:

#### Latest release

```powershell
iwr -useb "https://get.radapp.dev/tools/rad/install.ps1" | iex
```

#### Edge build

```powershell
$script=iwr -useb  https://radiuspublic.blob.core.windows.net/tools/rad/install.ps1; $block=[ScriptBlock]::Create($script); invoke-command -ScriptBlock $block -ArgumentList edge
```

#### Specific version

```powershell
$script=iwr -useb  https://get.radapp.dev/tools/rad/install.ps1; $block=[ScriptBlock]::Create($script); invoke-command -ScriptBlock $block -ArgumentList <Version>
```

{{% /codetab %}}
{{% codetab %}}

#### Latest release

```bash
curl -fsSL "https://get.radapp.dev/tools/rad/install.sh" | /bin/bash
```

#### Edge build

```bash
curl -fsSL "https://radiuspublic.blob.core.windows.net/tools/rad/install.sh" | /bin/bash -s edge
```

#### Specific version

```bash
curl -fsSL "https://get.radapp.dev/tools/rad/install.sh" | /bin/bash -s <Version>
```

{{% /codetab %}}

{{% codetab %}}

#### Latest release

```bash
wget -q "https://get.radapp.dev/tools/rad/install.sh" -O - | /bin/bash
```

#### Edge build

```bash
wget -q "https://radiuspublic.blob.core.windows.net/tools/rad/install.sh" -O - | /bin/bash -s edge
```

#### Specific version

```bash
wget -q "https://get.radapp.dev/tools/rad/install.sh" -O - | /bin/bash -s <Version>
```

{{% /codetab %}}

{{% codetab %}}

[Azure Cloud Shell](https://docs.microsoft.com/en-us/azure/cloud-shell/overview) is an interactive, authenticated, browser-accessible shell for managing Azure resources.

Azure Cloud Shell for bash doesn't have a sudo command, so users are unable to install Radius to the default `/usr/local/bin` installation path. To install the rad CLI to the home directory, run the following commands:

```bash
export RADIUS_INSTALL_DIR=./
wget -q "https://get.radapp.dev/tools/rad/install.sh" -O - | /bin/bash
```

You can now run the rad CLI with `./rad`.

PowerShell for Cloud Shell is currently not supported.

{{% /codetab %}}

{{% codetab %}}

#### Latest release

1. Download the `rad` CLI from one of these URLs:

   - MacOS: https://get.radapp.dev/tools/rad/0.9/macos-x64/rad
   - Linux: https://get.radapp.dev/tools/rad/0.9/linux-x64/rad
   - Windows: https://get.radapp.dev/tools/rad/0.9/windows-x64/rad.exe

1. Ensure the user has permission to execute the binary and place it somewhere on your PATH so it can be invoked easily.

#### Edge build

1. Download the `rad` CLI from one of these URLs:

   - MacOS: https://radiuspublic.blob.core.windows.net/tools/rad/edge/macos-x64/rad
   - Linux: https://radiuspublic.blob.core.windows.net/tools/rad/edge/linux-x64/rad
   - Windows: https://radiuspublic.blob.core.windows.net/tools/rad/edge/windows-x64/rad.exe

1. Ensure the user has permission to execute the binary and place it somewhere on your PATH so it can be invoked easily.

#### Specific version

1. Download the `rad` CLI from one of these URLs (replace `<version>` with your desired version):

   - MacOS: https://get.radapp.dev/tools/rad/<version\>/macos-x64/rad
   - Linux: https://get.radapp.dev/tools/rad/<version\>/linux-x64/rad
   - Windows: https://get.radapp.dev/tools/rad/<version\>/windows-x64/rad.exe

2. Ensure the user has permission to execute the binary and place it somewhere on your PATH so it can be invoked easily.

{{% /codetab %}}

{{< /tabs >}}
