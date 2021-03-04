---
type: docs
title: "Deploy a Radius application"
linkTitle: "Deploy an application"
description: "How to use the rad CLI to deploy an application into your Azure subscription"
weight: 40
---

You can find some examples to deploy in the `test/` folder. The best example to start with is at `test/frontend-backend/azure-bicep/template.bicep`.

```sh
go run cmd/cli/main.go deploy <path-to-.bicep file>
```