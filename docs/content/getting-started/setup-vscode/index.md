---
type: docs
title: "Setup Visual Studio Code with the Radius extension"
linkTitle: "Setup VSCode"
description: "How to setup VSCode with the Radius extension for easy application authoring"
weight: 20
---

Radius can be used with any text editor, but Radius-specific optimizations are available for [Visual Studio Code](https://code.visualstudio.com/). The Project Radius VSCode extension provides:
- Syntax highlighting
- Auto-completion
- Linting

## Pre-requisites

- [Visual Studio Code](https://code.visualstudio.com/)

## Install Radius extension

1. Download the [custom VSCode extension file](https://radiuspublic.blob.core.windows.net/tools/vscode/edge/rad-vscode-bicep.vsix)

1. Install the `.vsix` file
   - In VSCode, manually install the extension using the *Install from VSIX* command in the Extensions view command drop-down.
       
       <img src="./vsix-install.png" alt="Screenshot of installing a vsix extension" width=400>
   - You can also import this extension on the [command-line](https://code.visualstudio.com/docs/editor/extension-gallery#_install-from-a-vsix) with:

      ```bash
      code --install-extension rad-vscode-bicep.vsix
      ```

1. Disable the official Bicep extension if you have it installed. (Do NOT install the Bicep extension if you haven't already.)
   - Our custom extension needs to be responsible for handling `.bicep` files and you cannot have both extensions enabled at once.

<br /><a class="btn btn-primary" href="{{< ref create-environment.md >}}" role="button">Next: Create environment</a>
