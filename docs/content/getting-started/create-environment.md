---
type: docs
title: "Create a Radius RP environment"
linkTitle: "Create an environment"
description: "How to initialize the private Radius resource provider in your Azure subscription"
weight: 20
---

Radius deploys a private resource provider, or control plane, that your `rad` cli connects to when managing your radius applications.

As a one time operation deploy a radius environment into your Azure subscription through the following instructions:

## 1. Login to Azure

Use the `az` CLI to authenticate with Azure your Azure account

```sh
az login
```

## 2. Select your subscription

Use the `az` CLI specify your preferred subscription

```sh
az account set --subscription <SUB-ID>
```

## 3. Create a radius environment

```sh
go run cmd/cli/main.go env init azure -i
```

This will prompt you for information and then go off and run a bunch of command to create assets in your subscription.