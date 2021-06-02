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
  component   Manage components
  deploy      Deploy a RAD application
  deployments Manage deployments
  env         Manage environments
  help        Help about any command

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
