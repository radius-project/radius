---
type: docs
title: "Your first commit: prequisites"
linkTitle: "Prerequisites"
weight: 50
---

## Operating system

We support developing on macOS, Linux, and Windows with WSL. 

{{% alert title="Regarding windows" color="warning" %}}
We frequently use *nix-orentied tools like `make` and shell scripts. If you prefer to use Windows then get to know WSL, and a *nix shell like Bash or ZSH.

Most of the team develop on macOS so you will have an easier time getting help that way.
{{% /alert %}}

## Required Installs

This is the list of core dependencies to install for the most common tasks. In general we expect all contributors to have all of these tools present.

- Make
- [Go](https://golang.org/doc/install)
- [Node.js](https://nodejs.org/en/)
- [Golangci-lint](https://golangci-lint.run/usage/install/#local-installation)

{{% alert title="Package Managers" color="info" %}}
On our supported OSes using a package manager to install these dependencies is a much easier way to keep them updated. 

For macOS, this likely means you should be using [Homebrew](https://brew.sh/).

On Linux, use your distro's package manager.
{{% /alert %}}