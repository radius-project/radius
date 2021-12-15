---
type: docs
title: "Radius CLI reference"
linkTitle: "rad CLI"
description: "Detailed reference documentation on the Radius CLI"
weight: 100
---

```bash
$ rad

Usage:
  rad [command]

Available Commands:
  application Manage applications
  bicep       Manage bicep compiler
  completion  Generates shell completion scripts
  deploy      Deploy a RAD application
  env         Manage environments
  help        Help about any command
  resource    Manage resources
  version     Prints the versions of the rad cli

Flags:
      --config string   config file (default is $HOME/.rad/config.yaml)
  -h, --help            help for rad
  -v, --version         version for radius

Use "rad [command] --help" for more information about a command.
```

## Global flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `$HOME/.rad/config.yaml` | config file

## Available commands
