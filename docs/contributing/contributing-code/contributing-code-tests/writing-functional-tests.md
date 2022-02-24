---
type: docs
title: "Writing Radius functional tests"
linkTitle: "Writing functional tests"
description: "How to write Radius functional tests"
weight: 250
---

You can find the functional tests under `./test/functional`. A functional tests (in our terminology) is a test that interacts with real hosting enviroments (Azure, Kubernetes), deploys real applications and resources, and covers realistic or simulated user scenarios.

## Organization

Functional tests are organized using the following pattern:

> `/test/functional/<host>/<kind>`

For example a test for deploying an Azure Service Bus resource would be in `/test/functional/azure/resources/servicebus_test.go`. It's fine to create additional levels of hierarchy within the `<kind>`.

`.bicep` files used by tests should be in the `testdata` folder inside the test's package.

## Authoring

Tests should look like the following. You can actually copy-paste this to create a new test!

```go
func Test_DescriptiveTestName(t *testing.T) {
	application := "unique-application-name"
	template := "testdata/unique-application-name.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template, ""),
            Components: &validation.ComponentSet{
                // Set of components to validate
            },
			Pods: &validation.K8sObjectSet{
				// set of K8s resources to validate
			},
            // This should be set to true for every test for now
			SkipARMResources: true,
		},
	})

	test.Test(t)
}
```

When adding a new functional test:

- Follow established patterns for naming of things like application, template filename, test filename
- Double-check that the application name is unique (do a search in the repo)
- Avoid skipping any verifications (other than `SkipARMResources`)
- Avoid using `PostStepVerify` and `PostDeleteVerify` if you can add new capabilities to the test system
- For the tests to verify that the containers are actually started and in Ready state, you can add a readiness probe to the bicep file as below. 
	```
	resource a 'Container' = {
		name: 'a'
		properties: {
		container: {
			image: '${registry}/magpie:latest' // This image implements readiness checks
			env: {
			COOL_SETTING: env
			}
			readinessProbe:{
			kind:'httpGet'
			containerPort:3000
			path: '/healthz'
			}
		}
		}
	}
	```