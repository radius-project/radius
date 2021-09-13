---
type: docs
title: "Your first commit: Prequisites"
linkTitle: "Prerequisites"
description: "How to setup your system to begin developing for Radius"
weight: 50
---

## Operating system

We support developing on macOS, Linux and Windows. 

## Required installs

This is the list of core dependencies to install for the most common tasks. In general we expect all contributors to have all of these tools present:

- [Git](https://git-scm.com/downloads)

- Make
  
  **Windows**: Install make with [Chocolatey](https://chocolatey.org/install)
  ```cmd
  choco install make
  ```
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

{{% alert title="Package managers" color="primary" %}}
On our supported OSes using a package manager to install these dependencies is a much easier way to keep them updated. 
- For macOS, this likely means you should be using [Homebrew](https://brew.sh/).
- On Linux, use your distro's package manager.
{{% /alert %}}

{{< button text="Next step: Install development tools" page="first-commit-01-development-tools.md" >}}