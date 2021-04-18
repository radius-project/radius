---
type: docs
title: "Test Radius locally"
linkTitle: "Test locally"
description: "How to run integration tests on Radius locally"
weight: 30
---

# Testing locally

Testing the RP locally can be challenging because the Radius RP is just one part of a distributed system. The actual processing of ARM templates (the output of a `.bicep file`) is handled by the ARM deployment engine, not us.

For this reason we've built the `radtest` tool. This emulates some of the **basic** features of ARM templates in a CLI tool so that you can test without the central ARM infrastructure.

## Pattern for integration testing

As a general pattern, you can find example applications in the `/examples` folder. Each folder has a `template.bicep` file which contains a deployable application.

If you are building new features, or want to test deployment interactions the best way is to either:

- Make a series of deploy and delete operations with one of these example applications
- Write a new example application

## Ad-hoc testing with radtest

`radtest` is a Go CLI that you can run with `go run cmd/radtest/main.go`.

Examples:

```sh
# deploy the frontend-backend application
go run cmd/radtest/main.go deploy examples/frontend-backend/template.bicep

# delete the frontend-backend application
go run cmd/radtest/main.go delete examples/frontend-backend/template.bicep

# deploy the frontend-backend application and print all requests/response
go run cmd/radtest/main.go deploy examples/frontend-backend/template.bicep --verbose
```

You might want to make a wrapper script for this to make it more convenient:

```sh
#!/bin/sh
go run ~/github.com/Azure/radius/cmd/radtest/main.go $@
```