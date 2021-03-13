---
type: docs
title: "env delete CLI command reference"
linkTitle: "env delete"
description: "Detailed reference documentation on the Radius CLI env delete command"
weight: 20000
---

## Description

Delete a Radius environment. Note that this will delete:
- The environment itself
    - For Azure environments, this includes the resource group and anything else in it
- Any applications that were deployed into a environment, along with any data within the applications.




## Usage

```bash
rad env delete [flags]
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | | help for list
| `y`, `--yes` | `false` | Do not prompt for confirmation

Visit the [CLI page]({{< ref "cli#global-flags" >}}) for a list of available global flags.
