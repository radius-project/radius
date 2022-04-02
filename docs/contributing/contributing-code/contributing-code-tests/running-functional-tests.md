# Running Radius functional tests

You can find the functional tests under `./test/functional`. A functional tests (in our terminology) is a test that interacts with real hosting enviroments (Azure, Kubernetes), deploys real applications and resources, and covers realistic or simulated user scenarios.

These tests verify whether:

- That Radius environments can be created successfully.
- That Bicep templates of sample applications ca be deployed to the Radius environment. 

These run on Azure Radius environments (also called Radius test clusters) that are managed dynamically as part of the test process.

## Running via GitHub workflow

These tests automatically run for every PR in the `azure-pipelines.yml` github workflow.

We do not run these tests for commits to `main` or tags since they might block the build if they fail.

### How this works 

For each PR we run the following set of steps:

- Create a new test environment.
- Run deployment tests.
- Delete the environment.

## Configuration

These tests use your local Azure credentials, and Radius environment for testing. In a GitHub workflow, our automation makes the CI environment resemble a dev scenario.

The tests use our product functionality (the Radius config file) to configure the environment.

## Running the tests locally

1. Create an environment (`rad env init azure -i`)
2. Place `rad` on your path
3. Make sure `rad-bicep` is downloaded (`rad bicep download`)
4. Run:

    ```sh
    make test-functional-azure
    ```

When you're running locally with this configuration, the tests will use your locally selected Radius environment and your local copy of `rad`.

You can also run/debug individual tests from VSCode

### Seeing log output

Some of these tests take a few minutes to run since they interact with cloud resources. You should configure VSCode to output verbose output so you can see the progress.

Open your VSCode `settings.json` with the command `Preferences: Open Settings (JSON)` and configure the following options:
```
{
    ...
    "go.testTimeout": "60m",
    "go.testFlags": [
        "-v"
    ]
}
```
