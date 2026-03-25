# Organization of functional tests

* **Author**: Vinaya Damle (@vinayada1)

## Overview

Currently, all the functional tests require an approval to run which is unnecessary. Only, the tests that use cloud resources need to be approved to run. Also, in the future, we could have tests that require running in AKS/EKS cluster, use a mix of AWS and Azure resources. Currently, we do not have a good plan for organizing such tests.

This is a proposal for organization of functional tests such that it accounts for these requirements and logically organizes the test cases

## Terms and definitions

NA

## Objectives

> **Issue Reference:** https://github.com/radius-project/radius/issues/6588

### Goals

1. Eliminate the need of an approval for running tests that do not use cloud resources
2. Account for future tests that might require running in AKS/EKS cluster, a mixed use of AWS and Azure resources, etc.

### Non goals

NA


## Design

The proposal is to have the following directory structure:-
```
- test
    - functional-portable (run on any k8s clusters)
        - ucp
            - cloud
            - noncloud		
                - use features and a single folder for aws and azure
        - messagingrp
            - cloud
            - noncloud		
                - use features and a single folder for aws and azure
        - kubernetes
            - cloud
            - noncloud		
                - use features and a single folder for aws and azure
        - daprrp
            - cloud
            - noncloud		
                - use features and a single folder for aws and azure
        - datastoresrp
            - cloud
            - noncloud		
                - use features and a single folder for aws and azure
        - corerp
            - cloud
            - noncloud		
                - use features and a single folder for aws and azure
        - cli
            - cloud
            - noncloud		
                - use features and a single folder for aws and azure
        - samples
            - cloud
            - noncloud		
                - use features and a single folder for aws and azure
    - functional-aks (future)
    - functional-eks (future)
```

### Design details


A user, trying to add a new functional test, can use the logic below to determine where to add a new test:-
1. Identify if the tests can run on any k8s cluster or specifically requires running in AKS/EKS cluster.
2. Identify the area/feature that is being tested (e.g. ucp, samples, daprrp)
3. Determine if the test needs to use cloud resources
4. If cloud resources are used, whether AWS or Azure resources (or both in the future) are needed

### API design (if applicable)

NA

## Alternatives considered

1. Organize tests that require AWS and Azure resources into separate directories instead of using features. This will require a re-organization of tests if we decide to add tests with a mix of AWS and Azure resources
2. Invert the directory structure with cloud/non-cloud at the parent level and the area (ucp, daprrp, etc.) at the child level. However, it might make more sense for a user to approach this the other way when they are thinking about the feature being added.

## Test plan

NA

## Security

NA

## Compatibility (optional)

NA

## Monitoring

NA

## Development plan

- Move the tests to fit the directory structure in this proposal
- Modify the github workflow to always run the non-cloud tests but require an approval to run the tests using cloud resources
- Add notes to contributing documentation to guide future contributions

## Open issues

NA