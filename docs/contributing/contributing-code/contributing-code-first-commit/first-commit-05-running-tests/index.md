---
type: docs
title: "Your first commit: Running tests"
linkTitle: "Running tests"
description: How to run unit tests on your local machine
weight: 100
---

## Running tests

To run all the unit tests for the project from the command line:

```sh
make test
```

After tests run, you should see a big list of all of the project's packages:

```txt
go test ./pkg/...
ok  	github.com/project-radius/radius/pkg/radrp	0.328s
?   	github.com/project-radius/radius/pkg/radrp/armerrors	[no test files]
?   	github.com/project-radius/radius/pkg/radrp/certs	[no test files]
?   	github.com/project-radius/radius/pkg/radrp/db	[no test files]
?   	github.com/project-radius/radius/pkg/radrp/k8sauth	[no test files]
?   	github.com/project-radius/radius/pkg/radrp/metadata	[no test files]
ok  	github.com/project-radius/radius/pkg/radrp/resources	0.283s
?   	github.com/project-radius/radius/pkg/radrp/rest	[no test files]
?   	github.com/project-radius/radius/pkg/radrp/revision	[no test files]
ok  	github.com/project-radius/radius/pkg/cli	0.250s
?   	github.com/project-radius/radius/pkg/azure/azcli	[no test files]
?   	github.com/project-radius/radius/pkg/cli/azure	[no test files]
?   	github.com/project-radius/radius/pkg/cli/bicep	[no test files]
?   	github.com/project-radius/radius/pkg/cli/environments	[no test files]
?   	github.com/project-radius/radius/pkg/cli/logger	[no test files]
?   	github.com/project-radius/radius/pkg/cli/namegenerator	[no test files]
?   	github.com/project-radius/radius/pkg/cli/prompt	[no test files]
?   	github.com/project-radius/radius/pkg/cli/util	[no test files]
?   	github.com/project-radius/radius/pkg/azure/radclient	[no test files]
?   	github.com/project-radius/radius/pkg/renderers	[no test files]
ok  	github.com/project-radius/radius/pkg/renderers/containerv1alpha3
ok   	github.com/project-radius/radius/pkg/renderers/cosmosdbmongov1alpha3
ok   	github.com/project-radius/radius/pkg/renderers/dapr
ok   	github.com/project-radius/radius/pkg/renderers/daprpubsubv1alpha3
ok   	github.com/project-radius/radius/pkg/renderers/daprstatestorev1alpha3
ok   	github.com/project-radius/radius/pkg/renderers/servicebusqueuev1alpha3
```

The Go test tools do not make much fanfare when all the tests pass - it just says `ok` for every package that has tests.
In general it will be very obvious in the CLI output if something failed.

## Running/Debugging a single test

The best way to run a single test or group of tests is from VS Code.

Open `./pkg/rad/config_test.go` in the editor. Each test function has the options to run or debug the test right above it.

<img width="600px" src="unittest-commands.png" alt="Commands to launch for a unit test"><br />

{{< button text="Next step: Create a PR" page="first-commit-06-creating-a-pr.md" >}}
