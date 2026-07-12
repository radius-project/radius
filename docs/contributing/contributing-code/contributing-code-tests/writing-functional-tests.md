# Writing Radius functional tests

## Purpose

This guide explains how to add a functional test to the portable Radius test suite. Functional tests deploy real applications and resources to Kubernetes or a cloud provider and validate complete user scenarios. Use them when a change cannot be covered by the self-contained unit and integration tests that run through `make test`.

For instructions on running the suite, including its prerequisites and cleanup behavior, see [Running Radius functional tests](./running-functional-tests.md).

## Prerequisites

- Complete the setup in [Running Radius functional tests](./running-functional-tests.md#prerequisites).
- Choose the existing test group that owns the behavior you are testing. The Make targets and package paths for every group are defined in [`build/test.mk`](../../../../build/test.mk).
- Read a nearby test in the same group and follow its setup and validation patterns.

## Steps

### 1. Choose the package

Portable functional tests live under:

```text
test/functional-portable/<group>[/<cloud-or-noncloud>][/<kind>]
```

For example, non-cloud Core RP resource tests live under `test/functional-portable/corerp/noncloud/resources`, while upgrade tests live directly under `test/functional-portable/upgrade`. The group names generally match the `make test-functional-<group>` targets; the Messaging RP target is named `msgrp`, while its source directory is `messagingrp`. Use the exact package path in [`build/test.mk`](../../../../build/test.mk) for the group you are changing.

Put `.bicep` files and other fixtures in a `testdata` directory inside the test package.

### 2. Follow the current test harness

Most resource-provider tests use `rp.NewRPTest`, one or more `rp.TestStep` values, a deploy executor, and explicit resource or Kubernetes-object validation. A minimal test has this shape:

```go
package resource_test

import (
 "testing"

 "github.com/radius-project/radius/test/rp"
 "github.com/radius-project/radius/test/step"
 "github.com/radius-project/radius/test/validation"
)

func Test_DescriptiveTestName(t *testing.T) {
 name := "unique-test-name"
 template := "testdata/unique-test-name.bicep"

 test := rp.NewRPTest(t, name, []rp.TestStep{
  {
   Executor: step.NewDeployExecutor(template, ""),
   RPResources: &validation.RPResourceSet{
    Resources: []validation.RPResource{
     {
      Name: name,
      Type: validation.ApplicationsResource,
     },
    },
   },
  },
 })

 test.Test(t)
}
```

Copy a nearby test rather than this skeleton when the scenario needs recipe modules, cloud credentials, custom cleanup, output-resource validation, or Kubernetes assertions.

### 3. Keep the test isolated

- Give applications, environments, and resources names that are unique across the repository.
- Follow the [functional-test naming conventions](./tests-naming-conventions.md).
- Keep non-cloud tests independent of cloud accounts and cloud resources.
- Add readiness probes to test containers when the test needs to assert that a workload becomes ready.
- Prefer the existing validation sets over custom `PostStepVerify` or `PostDeleteVerify` callbacks. Add a callback only when the shared validation framework cannot express the assertion.
- Put cleanup in the test harness so failed tests do not leave resources behind.

### 4. Run the narrowest target

Run the package directly while iterating:

```bash
go test ./test/functional-portable/<package-path>/...
```

Then run its Make target before opening a pull request:

```bash
make test-functional-<group>-<cloud-or-noncloud>
```

## Verification

- The new test passes through both `go test` on its package and the matching `make test-functional-*` target.
- Test fixtures are under the package's `testdata` directory.
- Resource names follow the naming conventions and do not collide with another test.
- The test cleans up everything it creates.

## Troubleshooting

- **The deployment succeeds but validation fails.** Compare the `validation.RPResourceSet` or `validation.K8sObjectSet` with a nearby test for the same resource type.
- **A Terraform recipe test cannot find its module.** Publish the test recipes and set the module server URL as described in [Running Radius functional tests](./running-functional-tests.md).
- **The test passes alone but fails in a group.** Check for reused application, environment, namespace, or resource names.
