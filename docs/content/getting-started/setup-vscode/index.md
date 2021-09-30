---
type: docs
title: "Setup Visual Studio Code with the Radius extension"
linkTitle: "Setup tools"
description: "How to setup VS Code with the Radius extension for easy application authoring"
weight: 20
---

Radius can be used with any text editor, but Radius-specific optimizations are available for [Visual Studio Code](https://code.visualstudio.com/). The Project Radius VSCode extension provides:
- Syntax highlighting
- Auto-completion
- Linting

## Pre-requisites

- [Visual Studio Code](https://code.visualstudio.com/)

## Install Radius extension

1. Download the latest [custom VSCode extension file](https://get.radapp.dev/tools/vscode/stable/rad-vscode-bicep.vsix).

1. Install the `.vsix` file

   {{< tabs UI Terminal >}}
   
   {{% codetab %}}
   In VSCode, manually install the extension using the *Install from VSIX* command in the Extensions view    command drop-down.
          
   <img src="./vsix-install.png" alt="Screenshot of installing a vsix extension" width=400>
   {{% /codetab %}}
   
   {{% codetab %}}
   You can also import this extension on the [command-line](https://code.visualstudio.com/docs/editor/extension-gallery#_install-from-a-vsix) with:
   
   ```bash
   code --install-extension rad-vscode-bicep.vsix
   ```
   {{% /codetab %}}
   
   {{< /tabs >}}

1. Disable the official Bicep extension if you have it installed. (Do not install the Bicep extension if you haven't already, our custom extension needs to be responsible for handling `.bicep` files and you cannot have both extensions enabled at once.)

1. If running on Windows Subsystem for Linux (WSL), make sure to install the extension in WSL as well:

   <img src="./wsl-extension.png" alt="Screenshot of installing a vsix extension in WSL" width=400>


## Install other Radius extension versions

You can access other versions of the Radius extension from the following URLs:

- [Latest unstable](https://get.radapp.dev/tools/vscode/edge/rad-vscode-bicep.vsix)

{{< button text="Next: Create environment" page="create-environment.md" >}}