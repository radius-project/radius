---
type: docs
title: "env init azure CLI command reference"
linkTitle: "env init azure"
description: "Detailed reference documentation on the Radius CLI env init azure command"
weight: 100000
---

## Description

Create a RAD environment on Azure

## Usage

```bash
rad env init azure [flags]
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | | Print help for azure
| `-i`, `--interactive` | | Specify interactive to choose subscription and resource group interactively
| `--location string` | | The Azure location to use for the environment
| `--name` | `azure` | The environment name
| `--resource-group`  | | The resource group to use for the environment
| `--subscription-id` | | The subscription ID to use for the environment

Visit the [CLI page]({{< ref "cli#global-flags" >}}) for a list of available global flags.
