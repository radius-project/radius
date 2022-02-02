---
type: docs
title: "Visual Studio Code extension for Project Radius"
linkTitle: "Visual Studio Code"
description: "Overview of the Visual Studio Code extension"
weight: 200
---
Radius offers a *preview* Radius Visual Studio Code extension for Radius-specific optimizations which enables users a variety of features related to better manage applications across cloud and edge.

## Features

### View your deployed applications

View environments, applications and resources deployed in a tree view style in Visual Studio Code.

### View logs from container resources

The Radius extension helps you find information about your applications with Radius by streaming logs directly from the resource to the terminal window inside the VS Code IDE.

Using the tree view resource nodes found in the explorer section, you can access a action icon command to access this log stream and view logs related to the specific resource choosen.

## Install Radius extension

1. Download the edge [custom Bicep VSCode extension file](https://radiuspublic.blob.core.windows.net/tools/vscode/edge/rad-vscode-bicep.vsix).

1. Download the stable [Radius VSCode extension file](https://radiuspublic.blob.core.windows.net/tools/vscode/stable/rad-vscode.vsix).

1. Install both `.vsix` file

   {{< tabs UI Terminal >}}

   {{% codetab %}}
   In VSCode, manually install the extension using the *Install from VSIX* command in the Extensions view    command drop-down.

   <img src="./vsix-install.png" alt="Screenshot of installing a vsix extension" width=400>
   {{% /codetab %}}

   {{% codetab %}}
   You can also import this extension on the [command-line](https://code.visualstudio.com/docs/editor/extension-gallery#_install-from-a-vsix) with:

   ```bash
   code --install-extension rad-vscode-bicep.vsix
   code --install-extension rad-vscode.vsix
   ```

   {{% /codetab %}}

   {{< /tabs >}}

1. Disable the official Bicep extension if you have it installed. (Do not install the Bicep extension if you haven't already, our custom extension needs to be responsible for handling `.bicep` files and you cannot have both extensions enabled at once.)

1. If running on Windows Subsystem for Linux (WSL), make sure to install the extension in WSL as well:

   <img src="./wsl-extension.png" alt="Screenshot of installing a vsix extension in WSL" width=400>
