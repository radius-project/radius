---
type: docs
title: "Setup Visual Studio Code for Project Radius"
linkTitle: "Setup tools"
description: "How to setup Visual Studio Code with the Radius extension for easy application authoring"
weight: 20
---

Poject Radius can be used with any text editor, but Radius-specific optimizations are available for [Visual Studio Code](https://code.visualstudio.com/).

## Extension features

{{% alert title="Dual extensions" color="info" %}}
While Project Radius is still using a forked version of Bicep, two extensions will need to be installed: one for Bicep formatting/linting, and one for interaction with Radius resources. In a future release we will return to a single extension.
{{% /alert %}}

### Bicep extension fork

The custom Bicep extension provides a number of features to help author Bicep templates, including:

- Syntax highlighting
- Auto-completion
- Linting
- Template visualization

### Radius extension

The Radius VS Code extension provides users:

- Radius environment, application, and resource management
- Container log streaming

Learn more in the [extension documents]({{< ref vscode >}}).

## Installation

### Pre-requisites

- [Visual Studio Code](https://code.visualstudio.com/)

### Install Bicep and Radius extension

1. Download the latest [Bicep extension](https://get.radapp.dev/tools/vscode/stable/rad-vscode-bicep.vsix)

1. Download the latest [Radius extension](https://get.radapp.dev/tools/vscode/stable/rad-vscode.vsix)

1. Install both `.vsix` files:

   {{< tabs UI Terminal >}}

   {{% codetab %}}
   In VSCode, manually install the extension using the *Install from VSIX* command in the Extensions view command drop-down.

   <img src="./vsix-install.png" alt="Screenshot of installing a vsix extension" width=400>

   {{% /codetab %}}

   {{% codetab %}}
   You can also import this extension on the [command-line](https://code.visualstudio.com/docs/editor/extension-gallery#_install-from-a-vsix) with:

   ```bash
   curl https://radiuspublic.blob.core.windows.net/tools/vscode/edge/rad-vscode-bicep.vsix --output rad-vscode-bicep.vsix
   code --install-extension rad-vscode-bicep.vsix
   code --install-extension rad-vscode.vsix
   ```

   {{% /codetab %}}

   {{< /tabs >}}

1. Disable the official Bicep extension if you have it installed. Do not install it if prompted, our custom extension needs to be responsible for handling `.bicep` files and you cannot have both extensions enabled at once.

1. If running on Windows Subsystem for Linux (WSL), make sure to install the extension in WSL as well:

   <img src="./wsl-extension.png" alt="Screenshot of installing a vsix extension in WSL" width=400>

## Other versions

You can access other versions of the Radius Bicep extension fork from the following URLs:

- [Bicep edge](https://radiuspublic.blob.core.windows.net/tools/vscode/edge/rad-vscode-bicep.vsix)
- [Radius edge](https://radiuspublic.blob.core.windows.net/tools/vscode/edge/rad-vscode.vsix)

## Next step

{{< button text="Next: Create environment" page="create-environment.md" >}}
