---
type: docs
title: "Your first commit: Development tools"
linkTitle: "Development tools"
description: "Configuring Visual Studio Code for Radius development"
weight: 60
---

## Editor

This guide covers basic development tasks for Go in Visual Studio Code. The experience with VS Code is high-quality and approachable for newcomers.

Alternatively, you can choose whichever editor you are most comfortable for working on Go code. Feel free to skip this section if you want to make another choice.

## Installation

- [Visual Studio Code](https://code.visualstudio.com/)
- [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.go)

Install both of these and then follow the steps in the *Quick Start* for the Go extension.

The extension will walk you through an automated install of some additional tools that match your installed version of Go.

## Test it out

At this point you should be able to open any of the Go files in the repo and see syntax highlighting working.

{{% alert title="Launching VSCode" color="primary" %}}
The best way to launch VS Code for Go is to do *File* -> *Open Folder* on the repository. 

You can easily do this from the command shell with `code .`, which opens the current directory as a folder in VS Code.
{{% /alert %}}


{{< button text="Next step: Build Radius" page="first-commit-02-building.md" >}}

## Related links

- [Go in Visual Studio Code](https://code.visualstudio.com/docs/languages/go)