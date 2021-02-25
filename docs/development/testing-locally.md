# Testing locally

Testing the RP locally can be challenging because the Radius RP is just one part of a distributed system. The actual processing of ARM templates (the output of a `.bicep file`) is handled by the ARM deployment engine, not us.

For this reason we create tests both as `.bicep` files and as `.json` files (ARM payloads). This allows us to test the RP in isolation. 

## Pattern for integration testing

As a general pattern, you can find the tests in `test/`. 

Each folder has a `local/` folder with `.json` files and scripts.

Each folder has `azure-bicep/` folder with a `template.bicep` file. 

If you make changes or add a test, add parity between these two styles of test where possible. 

## Running tests locally

*We don't have a good way to test the actual `.bicep` files against a local RP. You should use the `local/` folder for local testing.

### Step 0: Install HTTPie

These tests use [HTTPie](https://httpie.io/) as an HTTP client.

### Step 1: Configure settings

You will need to configure some environment variables to run tests locally. The actual values of these aren't important, just that you set them.

```sh
export SUBSCRIPTION_ID="test-subscription"
export RESOURCE_GROUP="test-resource-group"
export RESOURCE_PROVIDER="test-resource-provider"
```

### Step 2: Run tests

Then just run the `deploy.sh` script to do deployment.

Run `delete.sh` to delete.