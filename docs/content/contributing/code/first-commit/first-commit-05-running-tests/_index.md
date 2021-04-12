---
type: docs
title: "Your first commit: running tests"
linkTitle: "Running tests"
description: How to run unit tests on your local machine
weight: 100
---

## Running tests

To run the all unit tests for the project from the command line:

```sh
$ make test
```

After tests run, you should see a big list of all of the project's packages:

```txt
go test ./pkg/...
ok  	github.com/Azure/radius/pkg/curp	0.328s
?   	github.com/Azure/radius/pkg/curp/armauth	[no test files]
?   	github.com/Azure/radius/pkg/curp/armerrors	[no test files]
?   	github.com/Azure/radius/pkg/curp/certs	[no test files]
?   	github.com/Azure/radius/pkg/curp/components	[no test files]
?   	github.com/Azure/radius/pkg/curp/db	[no test files]
?   	github.com/Azure/radius/pkg/curp/k8sauth	[no test files]
?   	github.com/Azure/radius/pkg/curp/metadata	[no test files]
ok  	github.com/Azure/radius/pkg/curp/resources	0.283s
?   	github.com/Azure/radius/pkg/curp/rest	[no test files]
?   	github.com/Azure/radius/pkg/curp/revision	[no test files]
ok  	github.com/Azure/radius/pkg/rad	0.250s
?   	github.com/Azure/radius/pkg/rad/azcli	[no test files]
?   	github.com/Azure/radius/pkg/rad/azure	[no test files]
?   	github.com/Azure/radius/pkg/rad/bicep	[no test files]
?   	github.com/Azure/radius/pkg/rad/environments	[no test files]
?   	github.com/Azure/radius/pkg/rad/logger	[no test files]
?   	github.com/Azure/radius/pkg/rad/namegenerator	[no test files]
?   	github.com/Azure/radius/pkg/rad/prompt	[no test files]
?   	github.com/Azure/radius/pkg/rad/util	[no test files]
?   	github.com/Azure/radius/pkg/radclient	[no test files]
?   	github.com/Azure/radius/pkg/workloads	[no test files]
ok  	github.com/Azure/radius/pkg/workloads/containerv1alpha1	0.214s
?   	github.com/Azure/radius/pkg/workloads/cosmosdocumentdbv1alpha1	[no test files]
?   	github.com/Azure/radius/pkg/workloads/dapr	[no test files]
?   	github.com/Azure/radius/pkg/workloads/daprcomponentv1alpha1	[no test files]
?   	github.com/Azure/radius/pkg/workloads/daprpubsubv1alpha1	[no test files]
?   	github.com/Azure/radius/pkg/workloads/daprstatestorev1alpha1	[no test files]
?   	github.com/Azure/radius/pkg/workloads/functionv1alpha1	[no test files]
?   	github.com/Azure/radius/pkg/workloads/ingress	[no test files]
?   	github.com/Azure/radius/pkg/workloads/servicebusqueuev1alpha1	[no test files]
?   	github.com/Azure/radius/pkg/workloads/webappv1alpha1	[no test files]
```

The Go test tools do not make much fanfare when all the tests pass - it just says `ok` for every package that has tests.
In general it will be very obvious in the CLI output if something failed.

## Running/Debugging a single test

The best way to run a single test or group of tests is from VS Code.

Open `./pkg/rad/config_test.go` in the editor. Each test function has the options to run or debug the test right above it.

<img width="600px" src="unittest-commands.png" alt="Commands to launch for a unit test"><br />

{{< button text="Next step: Create a PR" page="first-commit-06-creating-a-pr.md" >}}
