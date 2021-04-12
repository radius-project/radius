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

## Install rad CLI

1. Download the `rad` CLI from one of these links:

   - [MacOS](https://radiuspublic.blob.core.windows.net/tools/rad/edge/macos-x64/rad)
   - [Linux](https://radiuspublic.blob.core.windows.net/tools/rad/edge/linux-x64/rad)
   - [Windows](https://radiuspublic.blob.core.windows.net/tools/rad/edge/windows-x64/rad.exe)

1. Ensure the user has permission to execute the binary and place it somewhere on your PATH so it can be invoked easily.

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
