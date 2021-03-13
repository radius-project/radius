---
type: docs
title: "deploy CLI command reference"
linkTitle: "deploy"
description: "Detailed reference documentation on the Radius CLI deploy command"
weight: 1000
---

## Description

Deploy a RAD application

## Usage

```bash
rad deploy [app.bicep] [flags]
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-h`, `--help` | | Print help for deploy

Visit the [CLI page]({{< ref "cli#global-flags" >}}) for a list of available global flags.

## Example

### Deploy a Radius application using the CLI

```bash
rad deploy ./myapp.bicep
```