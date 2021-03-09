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

## 1. Install rad CLI

Download the `rad` CLI from one of these links:

- [MacOS](https://radiuspublic.blob.core.windows.net/tools/rad/edge/macos-x64/rad)
- [Linux](https://radiuspublic.blob.core.windows.net/tools/rad/edge/linux-x64/rad)
- [Windows](https://radiuspublic.blob.core.windows.net/tools/rad/edge/windows-x64/rad.exe)

Place this somewhere on your PATH so it can be invoked easily.

## 2. Install custom Bicep

You need a custom build of the `bicep` CLI. Using the distribution from azure/bicep **WILL NOT WORK**, you need this specific build.

Download from one of these links and add it to your path so it can be invoked by the `rad` CLI.

- [MacOS](https://radiuspublic.blob.core.windows.net/tools/macos-x64/bicep)
- [Linux](https://radiuspublic.blob.core.windows.net/tools/linux-x64/bicep)
- [Windows](https://radiuspublic.blob.core.windows.net/tools/windows-x64/bicep.exe)

## 3. Install custom VSCode extension

Install the VSCode extension from `.vsix` file.

- [Download](https://radiuspublic.blob.core.windows.net/tools/vscode/edge/rad-vscode-bicep.vsix)

Next you will need to disable the official Bicep extension if you have it installed. Our custom extension needs to be responsible for handling `.bicep` files and you cannot have both extensions enabled at once.