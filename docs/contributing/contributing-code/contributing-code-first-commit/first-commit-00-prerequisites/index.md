# Your first commit: Prerequisites

<!--
    Note: some of this content is synchronized with the prerequisites guide for simplicity. Keep these in sync!
-->

## Operating system

We support developing on macOS, Linux and Windows with [WSL](https://docs.microsoft.com/windows/wsl/install).

## Package managers

On our supported OSes using a package manager to install these dependencies is a much easier way to keep them updated.

- For macOS, this likely means you should be using [Homebrew](https://brew.sh/).
- On Linux, use your distribution's package manager.

## Required installs

We recommend the usage of either dev containers to setup your development environment. Here are the links that provide more details:

<!-- - [Getting started - GitHub Codespaces](../contributing-code-prerequisites/README.md#github-codespaces) -->
- [Getting started - Dev Containers](../contributing-code-prerequisites/README.md#vs-code-and-dev-container)

However, you can also install all tools locally. This is the list of core dependencies to install for the most common tasks. In general we expect all contributors to have all of these tools present:

- [Git](https://git-scm.com/downloads)
- [Go](https://golang.org/doc/install)
- [Node.js](https://nodejs.org/en/)
- [Python](https://www.python.org/downloads/)
- [Golangci-lint](https://golangci-lint.run/usage/install/#local-installation)
- [jq](https://jqlang.github.io/jq/download/)
- Make

### Install make

For `make` we advice the following installation steps depending on you OS.

#### Linux

Install the `build-essential` package:

```bash
sudo apt-get install build-essential
```

#### Mac

Using Xcode:

```bash
xcode-select --install
```

Using Homebrew:

```bash
brew install make
```

## Next step

- [Install development tools](../first-commit-01-development-tools/index.md)
