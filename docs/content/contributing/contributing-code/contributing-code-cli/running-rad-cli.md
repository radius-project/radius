---
type: docs
title: "Build the Radius CLI"
linkTitle: "Build CLI"
description: "How to build a local version of the Radius CLI"
weight: 10
---

For a lot of your development tasks you will need to build `rad` from source instead of using a binary.

This is the best way to test changes, and to make sure you have the latest bits (other people's changes).

## Creating an wrapper script (MacOS/Linux)

Create the following script and place it on your path with a name like `dev-rad`. 

```sh
#!/bin/sh
set -eu
go run ~/github.com/project-radius/radius/cmd/rad/main.go $@
```

Replace `~/github.com/project-radius/radius` with the path to your repository root.

Run `chmod +x dev-rad` to mark it executable

Now use it as-if it were `rad`

```txt
➜ dev-rad env
Project Radius CLI

Usage:
  rad [command]

Available Commands:
  deploy      Deploy a RAD application
  env         Manage environments
  expose      Expose local port
  help        Help about any command

Flags:
      --config string   config file (default is $HOME/.rad/config.yaml)
  -h, --help            help for rad

Use "rad [command] --help" for more information about a command.
```