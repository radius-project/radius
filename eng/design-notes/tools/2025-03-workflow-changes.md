# GitHub Workflow Changes

* **Author**: Brooke Hamilton

## Overview

Large GitHub workflows can be challenging to test and fix due to their complexity. While complexity itself is not a barrier to developing the workflows, the inability to debug and test on repo forks along with the difficulty in running workflow logic on local developer machines makes new features and bug fixes costly. The limited options for testing result in lengthy development times for simple changes.

This design describes an evolutionary approach to managing our GitHub workflows which will decrease the difficulty and time it takes to test changes. The design is expressed through a set of design principles that can be applied to all workflows.

This design advocates for three high-level principles:

* Workflows are testable from GitHub forks.
* Workflow logic is testable on a developer machine.
* Workflow logic is never duplicated.

## Terms and definitions

* Workflow: A configurable automated process made up of one or more jobs. Workflows are defined in YAML files stored in the .github/workflows directory.
* Job: A set of steps that execute as part of a workflow. Jobs can run sequentially or in parallel as defined in the workflow. Parallel jobs do not execute on the same runner, but they can share context using a workspace.
* Step: An individual task that can run commands, scripts, or actions. Steps are defined within jobs and are executed in the order they are listed.
* Context: A way to access information about workflow runs, variables, runner environments, jobs, and steps. Each context is an object that contains properties, which can be strings or other objects.
* Runner: A server that runs your workflows when they are triggered. GitHub provides hosted runners, or you can host your own.
* Action: A custom application for the GitHub Actions platform that performs a complex but frequently repeated task. Actions can be used as steps in workflows.
* Event: A specific activity that triggers a workflow. Examples include push, pull_request, and schedule.
* Artifact: Files created during a workflow run that can be saved and shared with other jobs or workflows.
* Matrix: A strategy that allows you to create multiple job runs for different combinations of variables, such as different versions of a programming language or operating system.
* Secrets: Encrypted environment variables that you create in a repository or organization. They are used to store sensitive information like API keys and tokens.
* Environment: A set of secrets, variables, and protection rules that are scoped to a specific environment, such as development, staging, or production.
* Core logic: the essential set of operations that represent the primary purpose and functionality of a workflow, i.e. the central processing that delivers the core value or outcome that the workflow was designed to achieve, separate from peripheral concerns like job definition, error reporting.

## Objectives

* Provide a reusable pattern for workflows so that developers can confidently and quickly modify workflows and create new ones.
* Provide guidelines for evaluating workflows during PR reviews.

### Goals

* Significantly reduce the time required to create/modify/fix GitHub workflows.
* Increase the reliability of workflows.
* Reduce time dedicated to addressing workflow failures by the on-call engineer.

### Non-goals

* This design does not include details on how each Radius workflow would be modified to align with the patterns defined here.
* Not every workflow must be updated to the principles defined in this design. We will prioritize refactoring workflows based on the level of effort each workflow is requiring from the team when we need to make changes or when the workflows fail and need attention.
* Replacement of security tokens, e.g. PATs, with GitHub applications.

### User scenarios

#### User story: Forks

As a Radius developer, I can fork a repo and run any workflow on the fork with minimal setup, so that I can quickly test changes and confidently submit a PR for those changes.

#### User story: Local debugging

As a Radius developer, I can debug and test workflow logic on my machine, without having to commit the workflow changes to a repository, so that I can efficiently finish my work and ensure high quality code in the workflows.

#### User story: Security context

As a Radius developer, I can establish a security context that works on my machine or within a GitHub workflow, so that I can easily test automation developed locally, and without code changes I can run that automation within a GitHub workflow.

#### User story: Review workflow process

As a PR reviewer, I can checkout the PR to my own fork and test workflows there so that I can ensure the quality of the workflow.

### Developer Experience

Developers should be able to follow a standard development process when modifying GitHub workflows. Reviewers can test by checking out the PR to their own fork.

```mermaid
---
title: Developer and PR Reviewer Experience
---
flowchart LR
    subgraph Developer
        direction TB
        fork["Fork the repo"]
        clone["Clone and branch"]
        code["Modify code"]
        testlocal["Test on dev machine"]
        pushtofork["Test on GitHub fork"]
        pr["Create PR"]

        fork --> clone
        clone --> code
        code --> testlocal
        testlocal --> pushtofork
        pushtofork --> pr
    end

    subgraph Reviewer
        direction TB
        reviewpr["Checkout PR to Fork"]
        testonfork["Test on Fork"]
        prfeedback["Provide Feedback"]
        reviewpr --> testonfork
        testonfork --> prfeedback
    end

    Developer --> Reviewer
    %%Reviewer --> Developer
```

_Fig. 1: The developer and PR reviewer steps_

## Design

### High Level Workflow Design Principles

#### Use GitHub workflows for CI/CD setup and workflow layout

We will use GitHub workflows for these things:

* Identity and Security
* Test runner setup
* Control flow, including matrix operations

GitHub workflows will not contain core logic (unless the workflow is very simple).

#### Core logic is debuggable and testable on a developer machine with minimal setup

```mermaid
flowchart TD
    ghsetup["GitHub Workflow Setup"]:::io
    devsetup["Dev Machine Setup"]:::io
    subgraph logic["Core Logic"]
        cloudsetup["Cloud Setup"]
        work["Do Work"]
        cloudcleanup["Cloud Cleanup"]
    end
    ghcleanup["GitHub Workflow Cleanup"]:::io
    devcleanup["Dev Machine Cleanup"]:::io
    
    ghsetup --> logic
    devsetup --> logic
    cloudsetup --> work
    work --> cloudcleanup
    logic --> ghcleanup
    logic --> devcleanup

    classDef io stroke-dasharray: 5 5
```

_Fig. 2: Testable automation that can run from a GitHub workflow or a dev machine_

Here is an example of logic that is embedded within a workflow, which prevents it from being tested on a developer machine:

```yaml
    - name: Publish UDT types
        if: steps.skip-build.outputs.SKIP_BUILD != 'true'
        run: |
          mkdir ./bin
          cp ./dist/linux_amd64/release/rad ./bin/rad
          chmod +x ./bin/rad
          export PATH=$GITHUB_WORKSPACE/bin:$PATH
          which rad || { echo "cannot find rad"; exit 1; }
          rad bicep download
          rad version
          rad bicep publish-extension -f ./test/functional-portable/dynamicrp/noncloud/resources/testdata/testresourcetypes.yaml --target br:${{ env.TEST_BICEP_TYPES_REGISTRY}}/testresources:latest --force
```

An updated workflow step might look like this.

```yaml
    - name: Publish UDT types
        if: steps.skip-build.outputs.SKIP_BUILD != 'true'
        run: make workflow-udt-tests-publish-types
```

The above `make` command would encapsulate the logic in the first example above, making it runnable from a dev machine. A composite make command would run the above step along with the other steps in the workflow.

#### Workflows can be run on any fork without requiring access to the `radius-project` repos, and without having a Radius GitHub security role.

```mermaid
flowchart LR
    fork["Fork the repo"]
    setupCreds["Set up credentials"]
    invoke["Invoke the forked workflow"]
    
    fork --> setupCreds
    setupCreds --> invoke
```

_Fig. 3: Running a GitHub workflow on a fork_

#### Use the GitHub CLI for default security context

On a developer machine, if the dev is logged into the GitHub CLI, the security context is automatically passed to GitHub by the CLI. For any action that is taken in GitHub we should default to using the GitHub CLI so that this context works locally and within workflows.

In some cases we will choose to use a GitHub CLI call instead of a GitHub provided workflow action so that we can have runnable and testable code on a developer machine (because GitHub actions are not locally testable.)

#### Use GitHub actions for setup, cleanup, and matrix operations

The use of actions should be limited to setup and cleanup that is unique to GitHub workflows or runners because any logic that exists in a GitHub action cannot be executed on a developer machine. Some examples are below:

* Cloning repos
* Installing tools
* Retrieving stored secrets
* Publishing results

> NOTE: Most of the items in the list above also happen on developer machines, but the implementation is different. For example, a dev machine is more likely to have a long-lived git repo, whereas a GitHub workflow will clone a repo for each workflow run. 

:warning: When using GitHub actions that are published on other repositories, we are [placing trust](https://arstechnica.com/information-technology/2025/03/supply-chain-attack-exposing-credentials-affects-23k-users-of-tj-actions/) in the authors of that repo that they will prevent malicious code from executing. Choose wisely, and consider forking and customizing a GitHub action instead of calling it directly. Our trust in GitHub actions is similar to the way we trust code dependencies: we trust our code the most, we highly trust code published by GitHub, Microsoft, CNCF, or other trusted entities, we mostly trust code created by people we trust, and least of all trust code created by people we do not know.

#### Reusable logic exists in custom GitHub actions instead of copy/pasted to multiple workflows

Apply the DRY principle (don't repeat yourself). Consider using [custom actions](https://docs.github.com/en/actions/sharing-automations/creating-actions/about-custom-actions), [reusable workflows](https://docs.github.com/en/actions/sharing-automations/reusing-workflows) and [composite actions](https://docs.github.com/en/actions/sharing-automations/creating-actions/creating-a-composite-action) to [avoid duplication](https://docs.github.com/en/actions/sharing-automations/avoiding-duplication) of the same logic in multiple workflows.

#### Core logic is runnable from Make commands

Any core logic that is currently in GitHub workflows will be moved to Make commands that are runnable on a developer machine (with some environment setup).

Developer automation is already provided through Make. Some of the GitHub workflows also use the same Make commands, e.g. `make build` is invoked from GitHub workflows.

Some of the existing Make commands invoke scripts. This pattern will continue in order to keep all significant logic isolated to scripts because we do not want to move the complexity problem from GitHub workflows into Make. Simple composite commands should remain within the Make files as they are today.

#### Configuration is provided through environment variables

Environment variables can be set on developer machines (and stored in `.env` files). GitHub workflows will set environment variables during setup steps. Core logic will read the environment variables.

#### Workflow schedules do not trigger on forks

Some workflows have scheduled executions. It does not make sense for these schedules to be triggered on forks; running scheduled workflows on forks only wastes compute time of the runners.

#### All workflows can run on dispatch and run on branches

All workflows can be manually triggered, in addition to the other ways they can be triggered, like on pull requests and on a schedule. Allowing manual triggers enables testing from forks.

All workflows can be run on branches so that fork owners do not have to merge the changes to the main branch of the fork. Having to run forks from the main branch makes fork synchronization more difficult.

#### Workflows that perform PR checks should be configurable to run from the committing branch

When a PR includes a change to a workflow, the workflow should be runnable from the branch if the person running the workflow has write access to the repo. (PRs submitted from forks where the contributor does not have write access should continue to require approval to run by someone with write access.)

#### Workflow names should match yaml file names

This is a simple way to prevent confusion when trying to match items in the list of workflows on the GitHub Actions tab with the files in the git repo.

### Design options considered but not chosen

* Adopt a new automation tool like [Just](https://github.com/casey/just) or [Task](https://taskfile.dev/), and deprecate Make. These tools have advantages, but a side-effect of adopting a new tool would likely be the existence of a new tool in our toolbox without the removal of Make, which would increase complexity. Make currently meets our needs for invoking developer automation.
* Remove everything from GitHub workflows except the invocation of another tool: GitHub workflows have advantages when setting up the automation environment, e.g. retrieving credentials, cloning repos, configuring runners, installing tools, etc.

## Security

### Least Privilege

Workflows will continue to adhere to the least privilege feature in which they declare which permissions are required by the workflow.

### Identity

Where possible, the automation logic will use the GitHub CLI, which has identity built in. Developers would log into the CLI when running code on their machines, which would allow them to run the logic on their forks. When running as part of at GitHub workflow, the GitHub CLI context is already set up and no further configuration is required.

## Development plan

### Docs: Contributing Automation Code

* A new folder exists in the `radius/docs` folder that provides a `README.md` file with guidance on creating and editing GitHub workflows and other repo automation like `make`.
* The `README.md` document contains a real-world example of converting complex workflow logic into a `make` command that invokes a shell script.
* The `README.md` document contains a checklist for reviewing workflow edits. Examples of checklist items are: 

    [ ] Workflow logic is testable on a developer machine by running `make` commands and with minimal setup.
    [ ] The workflow is runnable from a repo fork using `workflow_dispatch` (manually invoked).
    [ ] Workflow schedules do not trigger on forks.
    [ ] The GitHub CLI is used for GitHub operations.
    [ ] Logic is not copy/pasted in multiple workflows.
    [ ] Configuration is provided through environment variables.
    [ ] The workflow file name is the same as the display name.

### PR Template

Modify the PR template checklist for each repository that contains workflows to include items similar to the following:

Workflow modifications have been tested on a repo fork and the changes conform to the workflow development guidance (add hyperlink to the guidance )  

* [ ] This PR contains no modifications to GitHub workflows or repo automation.  
* [ ] Changes to GitHub workflows, actions, and other repo automation have been tested on a repo fork.

### Existing GitHub Workflows

Existing workflows will evolve to implement the design principles. For example, if a new step is being added to an existing workflow, the step would be added via a Make command, which adheres to the principle that all logic is runnable on a developer machine.

### New GitHub Workflows

New GitHub workflows will adhere to the design principles.

## Open Questions and Actions

* Which actions cannot be performed by the GitHub CLI? How will identity be provided to them?
* Provide guidance on which git/GitHub CLI commands allow a PR reviewer to check out a PR to their own fork.
* We need a prototype of moderate complexity that implements these principles.
* Can we separate the scheduling from the workflows so that forks do not automatically run the workflows?

## Design Review Notes
| Action | Follow-Up |
| ------ | --------- |
| Add an example of moving logic into a Make command and show the before and after. | Added a task to the development plan. |
| Consider [reusable workflows](https://docs.github.com/en/actions/sharing-automations/reusing-workflows). | Added links to the principles section. |
| Create a checklist for reviewing workflows. | Added a checklist to the development plan. |
| Workflow names should match the file names. | Added a principle. |
