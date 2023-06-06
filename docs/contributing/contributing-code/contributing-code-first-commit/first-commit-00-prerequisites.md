# Your first commit: Prerequisites

<!--
    Note: some of this content is synchronized with the prerequisites guide for simplicity. Keep these in sync!
-->

## Operating system

We support developing on macOS, Linux and Windows with [WSL](https://docs.microsoft.com/windows/wsl/install).

## Required installs

This is the list of core dependencies to install for the most common tasks. In general we expect all contributors to have all of these tools present:

- [Git](https://git-scm.com/downloads)

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
- [Go](https://golang.org/doc/install)
- [Node.js](https://nodejs.org/en/)
- [Golangci-lint](https://golangci-lint.run/usage/install/#local-installation)

## Package managers
On our supported OSes using a package manager to install these dependencies is a much easier way to keep them updated. 
- For macOS, this likely means you should be using [Homebrew](https://brew.sh/).
- On Linux, use your distro's package manager.

## Next step
- [Install development tools](first-commit-01-development-tools.md)
