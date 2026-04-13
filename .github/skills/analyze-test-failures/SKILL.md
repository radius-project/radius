---
name: analyze-test-failures
description: 'Analyze functional test failures from GitHub Actions CI runs. Use when: a PR has failing cloud or non-cloud functional tests, you need to identify root cause of test failures, you want to understand why a scheduled test run failed, or you need to correlate test failures with container/pod logs.'
argument-hint: 'Optional: PR number, workflow run ID, or leave blank to auto-detect from current branch'
---

# Analyze Functional Test Failures

Investigate and diagnose failures in Radius functional test CI runs using GitHub MCP tools for deep, AI-powered root cause analysis.

## Overview

The Radius project has two functional test workflows:

- **Cloud tests** (`functional-test-cloud.yaml`, workflow ID `108656603`): Tests requiring cloud resources (Azure, AWS). Triggered by `pull_request_target`, `schedule`, `workflow_dispatch`, `repository_dispatch`.
- **Non-cloud tests** (`functional-test-noncloud.yaml`, workflow ID `91296019`): Tests using only local resources (KinD cluster). Triggered by `pull_request`, `schedule`, `workflow_dispatch`, `repository_dispatch`.

### Cloud Test Jobs
- `ucp-cloud` — UCP cloud resource tests
- `corerp-cloud` — Core RP cloud tests (recipes, Azure resources)

### Non-Cloud Test Jobs
- `msgrp-noncloud` — Messaging RP tests
- `corerp-noncloud` — Core RP local tests
- `cli-noncloud` — CLI functional tests
- `samples-noncloud` — Sample application tests
- `kubernetes-noncloud` — Kubernetes integration tests
- `ucp-noncloud` — UCP local tests
- `upgrade-noncloud` — Upgrade path tests
- `dynamicrp-noncloud` — Dynamic RP tests
- `datastoresrp-noncloud` — Datastores RP tests
- `daprrp-noncloud` — Dapr RP tests

### Artifacts Uploaded Per Run
Each test job uploads these artifacts:
- `{name}_container_logs` — Kubernetes container stdout/stderr logs
- `{name}_recipes-pod-logs` — Terraform recipe server pod logs
- `{name}-radius-pod-logs` (non-cloud only) — Radius system pod logs
- `functional_test_results_{name}` (on failure) — jUnit XML test results from `gotestsum`
- `rad_cli_linux_amd64` — The `rad` CLI binary used for testing

### Shell Script Companion
There is also a shell script at `hack/analyze-test-failures.sh` for quick command-line analysis:
```bash
make analyze-test-failures PR=1234
```

## Procedure

### Step 1: Identify the Workflow Run

Determine which workflow run(s) to analyze based on user input.

**If a PR number is provided:**
1. Use `github-mcp-server-actions_list` with method `list_workflow_runs` for both workflow files, filtering by the PR's branch
2. Find the most recent run for each workflow

**If a run ID is provided:**
1. Use `github-mcp-server-actions_get` with method `get_workflow_run` to get run details directly

**If auto-detecting from branch:**
1. Run `git rev-parse --abbrev-ref HEAD` to get the current branch
2. Run `gh pr list --head <branch> --json number --limit 1` to find the associated PR
3. Then proceed as with a PR number

### Step 2: List Jobs and Find Failures

1. Use `github-mcp-server-actions_list` with method `list_workflow_jobs` and the run ID
2. Parse the jobs list to identify any with `conclusion: "failure"`
3. If no failures exist, report that all tests passed
4. Report the list of failed jobs with their names and URLs

### Step 3: Download Failed Job Logs

For each failed job:

1. Use `github-mcp-server-get_job_logs` with the job ID and `return_content: true`
2. Set `tail_lines` appropriately — start with 500 lines; increase if needed for context
3. Save the log content for analysis

### Step 4: Analyze Job Logs

Look for these patterns in the job logs, in priority order:

1. **Go test failures**: Lines matching `--- FAIL:` or `FAIL\t` — these identify which test functions failed and their duration
2. **Test error messages**: Lines immediately following `--- FAIL:` often contain the assertion failure or error message
3. **Panics**: `panic:` followed by stack traces (`goroutine N [running]:`) — indicates a crash in Radius code
4. **Timeouts**: `context deadline exceeded`, `test timed out after`, `i/o timeout` — indicates slow or hung operations
5. **Kubernetes errors**: `CrashLoopBackOff`, `OOMKilled`, `ImagePullBackOff` — indicates infrastructure issues
6. **Radius-specific errors**: `level.*error`, `"error":`, `failed to` — application-level errors in Radius components

### Step 5: Analyze Artifacts (If Needed)

If job logs alone don't explain the failure:

1. Use `github-mcp-server-actions_list` with method `list_workflow_run_artifacts` to list artifacts
2. Look for `container_logs` and `pod-logs` artifacts for the failed job
3. These contain Kubernetes pod logs from the `radius-system` namespace and test namespaces
4. Container logs often reveal Radius component crashes, resource provisioning failures, or configuration issues

**Important — dig into symptoms, don't just report them:**

- When pods show `ImagePullBackOff` or `Init:ImagePullBackOff`, always check K8s events for the **specific** pull error (rate limiting, auth failure, missing tag, wrong registry, etc.) — don't just report the symptom.
- When pods show `CrashLoopBackOff`, read the actual container logs to find the panic, fatal error, or exit reason — don't just say "pod is crash-looping".
- When pods show `OOMKilled`, note the memory limit and the container that was killed — this distinguishes between "limit too low" and "memory leak".
- When pods are `Pending` with `FailedScheduling`, report the specific scheduling constraint that failed (taints, resource requests, node affinity, etc.).

### Step 6: Cross-Reference and Diagnose

Perform deeper analysis:

1. **Correlate test failures with container errors**: If a test failed with a timeout, check container logs for the corresponding Radius component to see if it crashed or had errors during that time window
2. **Identify flaky tests**: Use `github-mcp-server-actions_list` to check recent runs of the same workflow — if the same test fails intermittently, it may be flaky
3. **Check for infrastructure issues**: If multiple unrelated tests fail, the issue may be infrastructure (cluster setup, networking) rather than code
4. **Map errors to code**: Use grep/view tools to find the failing test function in the codebase and understand what it tests

### Step 7: Report Findings

Present a structured summary:

```
## Functional Test Failure Analysis

**PR:** #1234 (branch-name)
**Run:** [Cloud Tests](url) — failure
**Run:** [Non-Cloud Tests](url) — success

### Failed Jobs

#### corerp-cloud (Run ucp-cloud functional tests)
**Failed Tests:**
- `Test_CosmosDB_Recipe` — context deadline exceeded after 60s
- `Test_Azure_Redis_Recipe` — expected status 200, got 500

**Root Cause Analysis:**
The CosmosDB recipe test timed out waiting for the recipe to complete deployment.
Container logs for `radius-appcore` show repeated errors:
"failed to get recipe output: context deadline exceeded"

This suggests the recipe execution is taking longer than the test timeout (60m).
The Azure Redis test failure appears to be a cascading failure from the same issue.

**Suggested Actions:**
1. Check if CosmosDB recipe deployment time has increased
2. Look at recent changes to `pkg/corerp/handlers/recipe.go`
3. Consider increasing test timeout or adding retry logic
```

## Common Failure Categories

When analyzing failures, categorize them:

| Category | Indicators | Typical Cause |
|----------|-----------|---------------|
| **Test Logic Bug** | Assertion failures, wrong expected values | Code change broke expected behavior |
| **Timeout** | `context deadline exceeded`, `test timed out` | Slow external service, resource contention |
| **Panic/Crash** | `panic:`, stack trace | Nil pointer, index out of range, etc. |
| **Infrastructure** | `CrashLoopBackOff`, `OOMKilled`, multiple unrelated failures | Cluster issues, resource limits |
| **Flaky Test** | Same test passes/fails on reruns with no code change | Race condition, timing dependency |
| **Recipe Failure** | Recipe deployment errors in container logs | Terraform/Bicep recipe issue |
| **Configuration** | Missing env vars, wrong registry, auth errors | CI configuration or secrets issue |

## Tips

- The cloud workflow uses `pull_request_target` for security (access to secrets). The PR number is stored in the `PR_NUMBER` environment variable within the workflow.
- Non-cloud tests use `pull_request` trigger directly.
- Test results are in jUnit XML format generated by `gotestsum` with `--junitfile ./dist/functional_test/results.xml`.
- The `process-test-results` composite action transforms and uploads these XML files.
- Each test matrix entry runs `make test-functional-{name}` (e.g., `make test-functional-corerp-cloud`).
