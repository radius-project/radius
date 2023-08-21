# Repository Prerequisites

This page lists the prerequisites for working with the repository. Most contributors should start with the basic prerequisites. Depending on the task you need to perform, you may need to install more tools.

## Operating system

We support developing on macOS, Linux and Windows with [WSL](https://docs.microsoft.com/windows/wsl/install).

## Asking for help

If you get stuck installing any of our dependencies, please ask for help in our [forum](https://discordapp.com/channels/1113519723347456110/1115302284356767814).

## Basic Prerequisites

<!--
    Note: some of this content is synchronized with the first-commit guide for simplicity. Keep these in sync!
-->

### Required tools

This is the list of core dependencies to install for the most common tasks. In general we expect all contributors to have all of these tools present:

- [Git](https://git-scm.com/downloads)
- [Go](https://golang.org/doc/install)
- [Node.js](https://nodejs.org/en/)
- [Python](https://www.python.org/downloads/)
- [Golangci-lint](https://golangci-lint.run/usage/install/#local-installation)
- [jq](https://jqlang.github.io/jq/download/)  
- Make  
  
  **Linux**: Install the `build-essential` package:
  ```bash
  sudo apt-get install build-essential
  ```
  **Mac**:
  Using Xcode:
  ```bash  
  xcode-select --install
  ```
  Using Homebrew:
  ```bash  
  brew install make
  ```


#### Testing Required Tools

If you have not already done so, clone the repository and navigate there in your command shell.

You can build the main outputs using `make`:

```sh
make build && make lint
```

Running these steps will run our build and lint steps and verify that the tools are installed correctly. If you get stuck or suspect something is not working in these instructions please [open an issue](https://github.com/radius-project/radius/issues/new/choose).

### Editor

If you don't have a code editor set up for Go, we recommend VS Code. The experience with VS Code is high-quality and approachable for newcomers.

Alternatively, you can choose whichever editor you are most comfortable for working on Go code. Feel free to skip this section if you want to make another choice.

- [Visual Studio Code](https://code.visualstudio.com/)
- [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.go)

Install both of these and then follow the steps in the *Quick Start* for the Go extension.

The extension will walk you through an automated install of some additional tools that match your installed version of Go.

**Launching VSCode**

The best way to launch VS Code for Go is to do *File* -> *Open Folder* on the repository. 

You can easily do this from the command shell with `code .`, which opens the current directory as a folder in VS Code.

## Additional Tools

### Containers

[Docker](https://docs.docker.com/engine/install/) is required to build our containers.

### Kubernetes

The easiest way to run Radius is on Kubernetes. To do this you will need the ability to create a Kubernetes cluster as well as to install `kubectl` to control that cluster, you probably also want Helm to install things in your cluster. There are many ways to create a Kubernetes cluster that you can use for development and testing. If you don't have a preference we recommend `kind`.

- [Install kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)
- [Install Helm](https://helm.sh/docs/intro/install/)
- [Install Kind](https://kubernetes.io/docs/tasks/tools/#kind)

#### Troubleshooting Kubernetes

You might want tools that can help debug Kubernetes problems and understand what's going on in the cluster. Here are some recommendations from the team:

- [Lens (UI for Kubernetes)](https://k8slens.dev/)
- [VS Code Kubernetes Tools](https://marketplace.visualstudio.com/items?itemName=ms-kubernetes-tools.vscode-kubernetes-tools)
- [Stern (console logging tool)](https://github.com/stern/stern#installation)

### Dapr

Radius includes integration with [Dapr](https://docs.dapr.io/). To use work on these features, you'll need to install the Dapr CLI.

- [Dapr](https://docs.dapr.io/getting-started/install-dapr-cli/)

### Code Generation

Our code generation targets are used to update generated OpenAPI specs and generated Go code based on those OpenAPI specs. Additionally, some Go code is generated mocks or Kubernetes API types. 

If you were trying to run `make generate` and ran into an error, then one of the below is likely missing. 

Enter the following commands to install all of the required tools.

```sh
cd cadl && npm ci
npm install -g autorest
npm install -g oav
go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.1
go install github.com/golang/mock/mockgen@v1.5.0
```

### Test summaries

The default `go test` output can be hard to read when you have many tests. We recommend `gotestsum` as a tool to solve this. Our `make test` command will automatically use `gotestsum` if it is available.

- [gotestsum](https://github.com/gotestyourself/gotestsum#install)