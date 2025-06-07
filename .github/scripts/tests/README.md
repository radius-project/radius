# Test Scripts

This directory contains automated tests for the Radius project's bash scripts using the Bats (Bash Automated Testing System) framework.

## Overview

These tests validate the functionality of our CI/CD bash scripts to ensure they work correctly across different scenarios and edge cases.

## Prerequisites

- **Bats Core**: A testing framework for Bash scripts

```bash
# Install on macOS
brew install bats-core

# Install on Linux
sudo apt-get install bats-core
```

## Running Tests

To run all tests:

```bash
cd .github/scripts/tests
bats *.test.bats
```

To run a specific test file:

```bash
cd .github/scripts/tests
bats release-get-version.test.bats
```

## Creating New Tests

1. Create a new test file with the .test.bats extension
1. Follow the existing test patterns:

```bash
#!/usr/bin/env bats

@test "My test description" {
  # Test code here
  [ "$result" = "expected value" ]
}
```
