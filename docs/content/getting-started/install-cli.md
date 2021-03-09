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
- [Go](https://golang.org/dl/)

## 1. Clone repository

The `rad` CLI lives in this repo and can be built/run from source.

Begin by cloning the Radius repo onto your machine:

```sh
git clone https://github.com/Azure/radius
```

Now `cd` into the directory you just created:

```sh
cd radius
```

## 2. Build the rad CLI

Run the following command to build the CLI and ensure it runs:

```sh
go run cmd/cli/main.go
```

## 3. Install custom Bicep

You need a custom build of the `bicep` CLI. Using the distribution from azure/bicep **WILL NOT WORK**, you need this specific build.

Download from one of these links and add it to your path so it can be invoked by the `rad` CLI.

- [MacOS](https://radiuspublic.blob.core.windows.net/tools/macos-x64/bicep)
- [Linux](https://radiuspublic.blob.core.windows.net/tools/linux-x64/bicep)
- [Windows](https://radiuspublic.blob.core.windows.net/tools/windows-x64/bicep.exe)

## 4. Install custom VSCode extension

Install the VSCode extension from `.vsix` file.

- [Download](https://radiuspublic.blob.core.windows.net/tools/vscode/edge/rad-vscode-bicep.vsix)

Next you will need to disable the official Bicep extension if you have it installed. Our custom extension needs to be responsible for handling `.bicep` files and you cannot have both extensions enabled at once.