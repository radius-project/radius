# Getting Started

You can use the `rad` CLI to create an environment and deploy applications.

The `rad` CLI lives in this repo and can be built/run from source. 

```sh
go run cmd/cli/main.go
```

### Step 1: Login to Azure

```sh
az login
```

### Step 2: Create an Environment

```sh
go run cmd/cli/main.go env init azure -i
```

This will prompt you for information and then go off and run a bunch of command to create assets in your subscription.

### Step 3: Install Bicep

You need a custom build of the `bicep` CLI. Using the distribution from azure/bicep **WILL NOT WORK**, you need this specific build.

Download from one of these links and add it to your path so it can be invoked by the `rad` CLI.

**Download Bicep:**

MacOS: https://radiuspublic.blob.core.windows.net/tools/macos-x64/bicep

Linux: https://radiuspublic.blob.core.windows.net/tools/linux-x64/bicep

Windows: https://radiuspublic.blob.core.windows.net/tools/windows-x64/bicep.exe

**Download VSCode Extension:**

https://radiuspublic.blob.core.windows.net/tools/vscode-bicep.vsix

Install the VSCode extension from `.vsix` file. Using the distribution from azure/bicep **WILL NOT WORK**, you need this specific build.

Next, make sure to disable auto-update of VSCode extensions. If you leave auto-updates enabled then your copy of the extension will get overridden with the main one and stop working.

### Step 4: Deploy Something

You can find some examples to deploy in the `test/` folder. The best example to start with is at `test/frontend-backend/azure-bicep/template.bicep`.

```sh
go run cmd/cli/main.go deploy <path-to-.bicep file>
```