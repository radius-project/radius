# Running Radius tests

## Types of tests

We apply the [testing pyramid](https://martinfowler.com/articles/practical-test-pyramid.html) to divide our tests into groups for each feature.

- Unit tests: exercise functions and types directly
- Integration tests: exercise features working with dependencies
- Functional test (also called end-to-end tests): exercise features in realistic user scenarios

## Unit tests

Unit tests can be run with the following command:

```sh
make test
```

We require unit tests to be added for new code as well as when making fixes or refactors in existing code. As a basic rule, ideally every PR contains some additions or changes to tests.

Unit tests should run with only the [basic prerequisites](../contributing-code-prerequisites/) installed. Do not add external dependencies needed for unit tests, prefer integration tests in those cases.

## Integration tests

> 🚧🚧🚧 Under Construction 🚧🚧🚧
>
> We don't currently define targets for integration tests. However we **do** have tests that require optional dependencies, and thus should be moved from the unit test category to the integration tests.

## Functional tests

Functional tests have their own dedicated set of [instructions](./running-functional-tests.md).

Functional tests generally use Radius to deploy an application and then make assertions about the state of that application. We have our infrastructure for this including a "test application" (`magpiego`) and our own test framework for deployment and verification.

## Test infrastructure and frameworks

We prefer to write unit tests in a straightforward style, and make sure of [testify](https://github.com/stretchr/testify) for assertions.

We like the productivity benefits of [subtests](https://go.dev/blog/subtests) and [table-driven tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests) and apply these patterns where appropriate.

We measure code coverage as part of the PR process because it provides a useful insight into whether the right tests are being added.