---
type: docs
title: "Visual Studio Code extension"
linkTitle: "Visual Studio Code"
description: "Overview of the Project Radius Visual Studio Code extension"
weight: 200
---

Developers can use the *preview* Radius Visual Studio Code extension which offers a variety of features to help manage Radius applications across cloud and edge.

{{% alert title="Bicep extension" color="info" %}}
In addition to the Project Radius extension described in this page, a forked Bicep extension is also used for formatting and linting. This requirement will be removed in a future release. Visit the [getting started guide]({{< ref setup-vscode >}}) for instructions on installing the custom Bicep extension.
{{% /alert %}}

## Features

### Manage your environments, applications, and resources

View and interact with environments, applications and resources deployed to your Radius environments.

Simply open the Project Radius extension and the extension will list all the environmetns in your local config, along with all of the applications and resources deployed to them:

<img src="vscode-tree.png" alt="Screenshot of environments, applications, and resources listed inside of VS Code" width="600px">

### View logs from container resources

The Radius extension helps you debug your applications by streaming logs directly from a container resource to the terminal window inside the VS Code IDE.

From the Visual Studio command palette, select `Radius: Show Container Logs` and select your environment, application, and container:

<img src="vscode-logs.png" alt="Screenshot of viewing container logs" width="600px">

## Installation

1. Download the latest [VSCode extension file](https://get.radapp.dev/tools/vscode/stable/rad-vscode.vsix)

1. Install the `.vsix` file:

   {{< tabs UI Terminal >}}

   {{% codetab %}}
   In VSCode, manually install the extension using the *Install from VSIX* command in the Extensions view    command drop-down.

   <img src="./vsix-install.png" alt="Screenshot of installing a vsix extension" width=400>
   {{% /codetab %}}

   {{% codetab %}}
   You can also import this extension on the [command-line](https://code.visualstudio.com/docs/editor/extension-gallery#_install-from-a-vsix) with:

   ```bash
   code --install-extension rad-vscode.vsix
   ```

   {{% /codetab %}}
   {{< /tabs >}}

1. If running on Windows Subsystem for Linux (WSL), make sure to install the extension in WSL as well:

   <img src="./wsl-extension.png" alt="Screenshot of installing a vsix extension in WSL" width=400>

## Additional resources

- [Forked Bicep extension instructions]({{< ref setup-vscode >}})
- [Edge extension download](https://get.radapp.dev/tools/vscode/edge/rad-vscode.vsix)