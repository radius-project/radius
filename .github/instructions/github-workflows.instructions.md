---
applyTo: ".github/workflows/*.yml,.github/workflows/*.yaml"
description: Comprehensive guide for building testable, secure, and efficient CI/CD pipelines using GitHub Workflows with emphasis on fork-testability and local development workflow patterns.
---

# GitHub Workflows Best Practices for Radius

## Your Mission

As GitHub Copilot, you are an expert in designing and optimizing CI/CD pipelines using GitHub Workflows. Your mission is to assist developers in creating efficient, secure, reliable, and **testable** automated workflows for building, testing, and deploying applications. You must prioritize testability from forks, local development patterns, security best practices, and provide actionable, detailed guidance.

## Radius Workflow Design Principles

These principles are specific to the Radius project and must be applied when creating or modifying workflows:

### 1. **Core Logic Must Be Testable on Developer Machines**

- **Principle:** Complex workflow logic should be executable on a developer's local machine with minimal setup, not embedded directly in workflow YAML files.
- **Implementation Pattern:**
  - Use GitHub workflows for CI/CD setup, runner configuration, identity/security, and control flow
  - Extract core business logic into Make targets that invoke shell scripts
  - Keep only simple, straightforward operations directly in workflow steps
- **When to Extract Logic to Make:**
  - Multi-step operations that could benefit from local testing
  - Complex build, test, or deployment procedures
  - Logic that needs to be reused across multiple workflows
  - Operations that require environment-specific configuration

- **When Inline YAML is Acceptable:**
  - Single-command operations (e.g., `npm install`, `go build`)
  - GitHub-specific setup actions (checkout, cache, artifact upload/download)
  - Simple conditional checks
  - Environment variable assignments

- **Example Transformation:**

  **Before (embedded logic, not testable locally):**

  ```yaml
  - name: Publish UDT types
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

  **After (testable via Make):**

  ```yaml
  - name: Publish UDT types
    run: make workflow-udt-tests-publish-types
    env:
      TEST_BICEP_TYPES_REGISTRY: ${{ env.TEST_BICEP_TYPES_REGISTRY }}
  ```

- **Guidance for Copilot:**
  - When writing complex workflow steps, first ask if this logic should be in a Make target
  - Suggest creating Make targets for multi-step operations
  - Ensure Make targets are documented and accept configuration via environment variables
  - Make targets should invoke shell scripts (following shell.instructions.md) for complex logic, not contain complex logic themselves

### 2. **Workflows Must Be Testable from Forks**

- **Principle:** Contributors should be able to fork the repository and test workflow changes on their fork with minimal setup, without requiring access to the main repository's secrets or special permissions.

- **Fork-Friendly Trigger Configuration:**
  - **ALWAYS include `workflow_dispatch`** during development to enable manual testing
  - **IMPORTANT:** Comment out `workflow_dispatch` before merging to production for workflows that shouldn't be manually triggered in production
  - Support running on any branch, not just `main` or specific protected branches
  - Use conditional logic to handle missing secrets gracefully when running on forks

- **Development Phase Pattern:**

  ```yaml
  # yaml-language-server: $schema=https://json.schemastore.org/github-workflow.json
  ---
  name: build-and-test

  on:
    push:
      branches:
        - main
        - "release/**"
    pull_request:
      branches:
        - main
    # workflow_dispatch: # Enable during development, comment out for production
    #   inputs:
    #     debug_enabled:
    #       description: Enable debug mode
    #       required: false
    #       default: "false"
  ```

- **Production-Ready Pattern:**

  ```yaml
  # yaml-language-server: $schema=https://json.schemastore.org/github-workflow.json
  ---
  name: build-and-test

  on:
    push:
      branches:
        - main
        - "release/**"
    pull_request:
      branches:
        - main
    # workflow_dispatch is commented out for production workflows
  ```

- **Fork-Friendly Secret Handling:**

  ```yaml
  - name: Deploy to staging
    if: github.repository == 'radius-project/radius' && github.event_name != 'pull_request'
    env:
      DEPLOY_TOKEN: ${{ secrets.DEPLOY_TOKEN }}
    run: make workflow-deploy-staging
  ```

- **Guidance for Copilot:**
  - Include commented-out `workflow_dispatch` for all workflows with a note about uncommenting during development
  - Use repository checks (`github.repository == 'radius-project/radius'`) for steps requiring secrets
  - Provide helpful skip messages when operations can't run on forks
  - Ensure core functionality (build, test) works without secrets when possible

### 3. **Scheduled Workflows Must Not Trigger on Forks**

- **Principle:** Scheduled workflows waste fork runners' compute time and should only run on the main repository.

- **Pattern:**

  ```yaml
  # yaml-language-server: $schema=https://json.schemastore.org/github-workflow.json
  ---
  name: Scheduled

  on:
    schedule:
      - cron: "0 0 * * 0" # Weekly on Sunday at midnight
    # workflow_dispatch: # Enable during development only

  jobs:
    scheduled-task:
      # Only run on the main repository, not forks
      if: github.repository == 'radius-project/radius'
      runs-on: ubuntu-24.04
      steps:
        - name: Run scheduled task
          run: make workflow-scheduled-task
  ```

- **Guidance for Copilot:**
  - Always add `if: github.repository == 'radius-project/radius'` to jobs in scheduled workflows
  - Explain why schedules shouldn't run on forks when suggesting this pattern

### 4. **Workflow Names Must Match File Names**

- **Principle:** Prevents confusion when matching workflows in the GitHub Actions UI with files in the repository.

- **Pattern:**
  - File: `.github/workflows/build-and-test.yml`
  - Workflow name: `name: build-and-test`
  - File: `.github/workflows/deploy-production.yml`
  - Workflow name: `name: deploy-production`

- **Reusable Workflows Convention:**
  - Reusable workflows must start with double underscore `__` prefix
  - File: `.github/workflows/__reusable-build.yml`
  - Workflow name: `name: __reusable-build`

- **Guidance for Copilot:**
  - Strongly recommend matching file and workflow names
  - For reusable workflows, enforce the `__` prefix convention
  - Flag mismatches during PR reviews

### 5. **Use GitHub CLI for GitHub Operations**

- **Principle:** The GitHub CLI provides automatic authentication context that works both locally (when developer is logged in) and in workflows (via `GITHUB_TOKEN`).

- **When to Prefer GitHub CLI:**
  - Creating/updating issues or pull requests
  - Managing releases
  - Working with GitHub API
  - Operations that need to work identically locally and in CI

- **When to Use GitHub Actions:**
  - Workflow-specific setup (checkout, cache, artifact management)
  - Installing tools and dependencies on runners
  - Matrix operations and parallel execution
  - Retrieving stored secrets from GitHub

- **Example:**

  ```yaml
  - name: Create release
    run: |
      gh release create ${{ github.ref_name }} \
        --title "Release ${{ github.ref_name }}" \
        --notes "Release notes here"
    env:
      GH_TOKEN: ${{ github.token }}
  ```

- **Guidance for Copilot:**
  - Suggest GitHub CLI commands that can be wrapped in Make targets
  - Use standard GitHub actions for runner setup and artifact management
  - Explain the testability benefits when recommending CLI over actions

### 6. **Configuration via Environment Variables**

- **Principle:** All configuration should be provided through environment variables, enabling the same code to run in different contexts (local dev, CI, different environments).

- **Pattern:**
  - GitHub workflows set environment variables in setup steps
  - Make targets and shell scripts read environment variables
  - Local developers can set variables in `.env` files or shell config
  - Document required environment variables in README or workflow comments

- **Example:**

  ```yaml
  jobs:
    deploy:
      runs-on: ubuntu-24.04
      steps:
        - name: Set environment variables
          run: |
            echo "DEPLOY_ENV=staging" >> $GITHUB_ENV
            echo "DEPLOY_REGION=eastus" >> $GITHUB_ENV
            echo "APP_VERSION=${{ github.sha }}" >> $GITHUB_ENV

        - name: Deploy application
          run: make workflow-deploy
          env:
            DEPLOY_TOKEN: ${{ secrets.DEPLOY_TOKEN }}
  ```

- **Guidance for Copilot:**
  - Avoid hardcoding values in scripts
  - Document required environment variables
  - Provide sensible defaults where appropriate

### 7. **Avoid Logic Duplication**

- **Principle:** Apply DRY (Don't Repeat Yourself). Use reusable workflows, composite actions, and Make targets.

- **Hierarchy of Reusability:**
  1. **Make targets** - For logic that should work locally and in CI
  2. **Reusable workflows** (`__prefix.yml`) - For complete workflow patterns
  3. **Composite actions** - For GitHub-specific setup sequences
  4. **Shell scripts** - For complex business logic invoked by Make

- **Reusable Workflow Pattern:**

  ```yaml
  # .github/workflows/__reusable-test.yml
  # yaml-language-server: $schema=https://json.schemastore.org/github-workflow.json
  ---
  name: __reusable-test

  on:
    workflow_call:
      inputs:
        test-suite:
          required: true
          type: string
      secrets:
        test-token:
          required: false

  jobs:
    test:
      runs-on: ubuntu-24.04
      steps:
        - uses: actions/checkout@<SHA> # vX.Y.Z
        - name: Run tests
          run: make test-${{ inputs.test-suite }}
          env:
            TEST_TOKEN: ${{ secrets.test-token }}
  ```

  ```yaml
  # .github/workflows/ci.yml
  name: ci

  jobs:
    unit-tests:
      uses: ./.github/workflows/__reusable-test.yml
      with:
        test-suite: unit
  ```

- **Guidance for Copilot:**
  - Identify repeated patterns across workflows
  - Suggest extracting to reusable workflows with `__` prefix
  - Ensure reusable workflows are well-documented

## Core GitHub Workflows Concepts and Structure

### 1. Workflow Structure

- **Naming Conventions:** Use descriptive names matching file names (e.g., `build-and-test.yml` → `name: build-and-test`)
- **Triggers (`on`):**
  - Use appropriate events: `push`, `pull_request`, `schedule`, `repository_dispatch`, `workflow_call`
  - Comment out `workflow_dispatch` in production workflows (use during development only)
  - For scheduled workflows, add fork protection: `if: github.repository == 'radius-project/radius'`
- **Concurrency:** Use to prevent simultaneous runs and avoid race conditions
  ```yaml
  concurrency:
    group: ${{ github.workflow }}-${{ github.ref }}
    cancel-in-progress: true
  ```
- **Permissions:** Define explicitly following least privilege principle
  ```yaml
  permissions:
    contents: read
    pull-requests: write # Only if needed
  ```

### 2. Jobs

- **Principle:** Jobs represent distinct pipeline phases (build, test, lint, security-scan, deploy)

- **Job Dependencies:**

  ```yaml
  jobs:
    build:
      runs-on: ubuntu-24.04
      outputs:
        artifact_path: ${{ steps.package.outputs.path }}
      steps:
        - name: Build and package
          id: package
          run: make workflow-build

    test:
      needs: build
      runs-on: ubuntu-24.04
      steps:
        - name: Run tests
          run: make workflow-test

    deploy:
      needs: [build, test]
      if: github.ref == 'refs/heads/main'
      runs-on: ubuntu-24.04
      steps:
        - name: Deploy
          run: make workflow-deploy
  ```

- **Conditional Execution:**
  ```yaml
  deploy-staging:
    if: |
      github.repository == 'radius-project/radius' &&
      (github.ref == 'refs/heads/main' || github.ref == 'refs/heads/develop')
  ```

### 3. Steps and Actions

- **Action Versioning:** Always pin to full commit SHA for security and immutability

  ```yaml
  - name: Checkout code
    uses: actions/checkout@<SHA> # vX.Y.Z
    with:
      persist-credentials: false
      fetch-depth: 1
  ```

- **Trust Model for External Actions:**
  - **Highly trusted:** `actions/*`, `github/*` (GitHub official)
  - **Trusted:** Microsoft, CNCF, verified organizations
  - **Use with caution:** Community actions (consider forking and auditing)
  - **Avoid:** Unverified or unknown authors

- **Step Naming:** Use descriptive names for logs and debugging
  ```yaml
  - name: Install dependencies and build project
    run: make workflow-build-all
  ```

## Security Best Practices

### 1. Secret Management

- **Never hardcode secrets** - Use GitHub Secrets exclusively

  ```yaml
  - name: Deploy to cloud
    env:
      CLOUD_API_KEY: ${{ secrets.CLOUD_API_KEY }}
    run: make workflow-deploy
  ```

- **Environment-Specific Secrets:**

  ```yaml
  jobs:
    deploy-prod:
      environment:
        name: production
        url: https://prod.radius.dev
      steps:
        - name: Deploy
          env:
            PROD_SECRET: ${{ secrets.PROD_SECRET }}
          run: make workflow-deploy-production
  ```

- **Fork-Safe Secret Usage:**
  ```yaml
  - name: Upload to registry
    if: github.repository == 'radius-project/radius' && github.event_name == 'push'
    env:
      REGISTRY_TOKEN: ${{ secrets.REGISTRY_TOKEN }}
    run: make workflow-publish
  ```

### 2. OIDC for Cloud Authentication

- **Principle:** Use OpenID Connect for cloud providers (AWS, Azure, GCP) instead of long-lived credentials

- **Azure Example:**
  ```yaml
  - name: Azure Login
    uses: azure/login@<SHA> # vX.Y.Z
    with:
      client-id: ${{ secrets.AZURE_CLIENT_ID }}
      tenant-id: ${{ secrets.AZURE_TENANT_ID }}
      subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}
  ```

### 3. Least Privilege for GITHUB_TOKEN

- **Always set minimum permissions:**

  ```yaml
  permissions:
    contents: read # Default - read-only access
    pull-requests: write # Only when updating PRs
    checks: write # Only when updating check runs
  ```

- **Job-Level Overrides:**

  ```yaml
  permissions:
    contents: read # Workflow default

  jobs:
    test:
      permissions:
        contents: read # This job only reads
      steps:
        - run: make test

    update-pr:
      permissions:
        contents: read
        pull-requests: write # This job needs to update PRs
      steps:
        - run: make update-pr-status
  ```

### 4. Dependency and Security Scanning

- **Dependency Review:**

  ```yaml
  - name: Dependency Review
    uses: actions/dependency-review-action@<SHA> # vX.Y.Z
    if: github.event_name == 'pull_request'
  ```

- **Static Analysis (SAST):**
  ```yaml
  - name: Run CodeQL Analysis
    uses: github/codeql-action/analyze@<SHA> # vX.Y.Z
  ```

### 5. Secret Scanning Prevention

- Enable GitHub's built-in secret scanning
- Use pre-commit hooks (e.g., `git-secrets`)
- Never log secrets, even when masked
- Review workflow logs for accidental exposure

## Optimization and Performance

### 1. Caching

- **Effective Cache Keys:**

  ```yaml
  - name: Cache dependencies
    uses: actions/cache@<SHA> # vX.Y.Z
    with:
      path: |
        ~/.npm
        node_modules
      key: ${{ runner.os }}-node-${{ hashFiles('**/package-lock.json') }}
      restore-keys: |
        ${{ runner.os }}-node-
  ```

- **Cache for Multiple Package Managers:**
  ```yaml
  - name: Cache Go modules
    uses: actions/cache@<SHA> # vX.Y.Z
    with:
      path: ~/go/pkg/mod
      key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}
      restore-keys: |
        ${{ runner.os }}-go-
  ```

### 2. Matrix Strategies

- **Parallel Testing Across Environments:**
  ```yaml
  jobs:
    test:
      runs-on: ${{ matrix.os }}
      strategy:
        fail-fast: false
        matrix:
          os: [ubuntu-24.04, windows-latest, macos-latest]
          go-version: ["1.21", "1.22"]
      steps:
        - uses: actions/checkout@<SHA> # vX.Y.Z
        - uses: actions/setup-go@<SHA> # vX.Y.Z
          with:
            go-version: ${{ matrix.go-version }}
        - run: make test
  ```

### 3. Shallow Clones

- **Default Pattern:**

  ```yaml
  - uses: actions/checkout@<SHA> # vX.Y.Z
    with:
      fetch-depth: 1 # Shallow clone for speed
      persist-credentials: false # Don't persist credentials
      submodules: false # Don't fetch submodules unless needed
  ```

- **When Full History Needed:**
  ```yaml
  - uses: actions/checkout@<SHA> # vX.Y.Z
    with:
      fetch-depth: 0 # Full history for release tagging, changelog generation
  ```

### 4. Artifacts

- **Build Artifact Sharing:**

  ```yaml
  jobs:
    build:
      steps:
        - name: Build application
          run: make workflow-build

        - name: Upload build artifacts
          uses: actions/upload-artifact@<SHA> # vX.Y.Z
          with:
            name: build-artifacts
            path: ./dist
            retention-days: 7

    test:
      needs: build
      steps:
        - name: Download build artifacts
          uses: actions/download-artifact@<SHA> # vX.Y.Z
          with:
            name: build-artifacts
            path: ./dist

        - name: Run tests
          run: make workflow-test
  ```

## Testing in CI/CD

### 1. Unit Tests

- **Fast Feedback Loop:**

  ```yaml
  unit-tests:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@<SHA>
      - name: Run unit tests with coverage
        run: make test-unit

      - name: Upload coverage report
        uses: actions/upload-artifact@<SHA>
        with:
          name: coverage-report
          path: coverage/
  ```

### 2. Integration Tests

- **With Service Dependencies:**
  ```yaml
  integration-tests:
    runs-on: ubuntu-24.04
    services:
      postgres:
        image: postgres:15
        env:
          POSTGRES_PASSWORD: testpass
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
    steps:
      - uses: actions/checkout@<SHA>
      - name: Run integration tests
        run: make test-integration
        env:
          DATABASE_URL: postgresql://postgres:testpass@localhost:5432/testdb
  ```

### 3. End-to-End Tests

- **Against Staging Environment:**
  ```yaml
  e2e-tests:
    runs-on: ubuntu-24.04
    needs: deploy-staging
    steps:
      - uses: actions/checkout@<SHA>
      - name: Run E2E tests
        run: make test-e2e
        env:
          TEST_ENV_URL: ${{ needs.deploy-staging.outputs.staging_url }}
  ```

### 4. Test Reporting

- **Publish Test Results:**
  ```yaml
  - name: Upload test results
    if: always()
    uses: actions/upload-artifact@<SHA>
    with:
      name: test-results
      path: |
        test-results/
        screenshots/
        videos/
  ```

## Workflow Review Checklist

Use this checklist when reviewing workflow changes:

### Radius-Specific Principles

- [ ] Complex logic is extracted to Make targets (testable locally)
- [ ] Make targets invoke shell scripts for complex operations
- [ ] `workflow_dispatch` is commented out for production workflows
- [ ] Scheduled workflows have fork protection (`if: github.repository == 'radius-project/radius'`)
- [ ] Steps requiring secrets check repository context
- [ ] Workflow name matches file name (exactly)
- [ ] Reusable workflows use `__` prefix
- [ ] Configuration is via environment variables
- [ ] GitHub CLI is used for GitHub operations (when appropriate)
- [ ] No duplicated logic (DRY principle applied)

### Security

- [ ] Secrets accessed via `${{ secrets.NAME }}` only
- [ ] OIDC used for cloud authentication (where applicable)
- [ ] `GITHUB_TOKEN` permissions set to minimum required
- [ ] External actions pinned to full commit SHA
- [ ] External actions from trusted sources only
- [ ] Dependency scanning enabled
- [ ] Secret scanning enabled

### Performance

- [ ] Caching configured with effective keys
- [ ] Shallow clones used (`fetch-depth: 1`) unless full history needed
- [ ] Artifacts used for job-to-job data transfer
- [ ] Matrix strategy for parallel operations
- [ ] `retention-days` set appropriately for artifacts

### Testing

- [ ] Unit tests run early in pipeline
- [ ] Integration tests configured with service dependencies
- [ ] E2E tests run against appropriate environment
- [ ] Test reports uploaded as artifacts
- [ ] Tests can run on forks (without secrets where possible)

### Deployment

- [ ] Environment protection configured for sensitive deployments
- [ ] Manual approvals for production
- [ ] Rollback strategy documented and implemented
- [ ] Health checks validate deployments
- [ ] Fork protection on deployment jobs

### Structure

- [ ] Clear job names representing pipeline phases
- [ ] `needs` dependencies properly defined
- [ ] Conditional execution uses `if` appropriately
- [ ] Step names are descriptive
- [ ] Concurrency configured to prevent conflicts
- [ ] Timeout configured for long-running jobs

## Troubleshooting

### Workflow Not Triggering

1. Check `on` triggers match the event
2. Verify `paths`, `branches` filters are correct
3. For `workflow_dispatch`, ensure file is in default branch
4. Review `if` conditions that might skip execution
5. Check concurrency settings for blocking runs

### Permission Errors

1. Review `permissions` at workflow and job level
2. Verify secret access and environment configuration
3. For OIDC, check trust policy in cloud provider
4. Confirm repository context for fork-safe operations

### Cache Misses

1. Validate cache key uses `hashFiles()` correctly
2. Ensure `path` matches actual dependency location
3. Use `restore-keys` for fallback patterns
4. Check cache size limits

### Flaky Tests

1. Add explicit waits (avoid `sleep`)
2. Ensure test isolation and cleanup
3. Use stable selectors for E2E tests
4. Implement retries for transient failures
5. Capture screenshots/videos on failure

### Fork Testing Issues

1. Verify `workflow_dispatch` is uncommented for testing
2. Check repository-specific `if` conditions
3. Ensure Make targets work without secrets
4. Test with minimal environment setup

## GitHub Actions Workflow Review Checklist (Comprehensive)

This checklist provides a granular set of criteria for reviewing GitHub Actions workflows to ensure they adhere to best practices for security, performance, and reliability.

- [ ] **General Structure and Design:**
  - Is the workflow `name` clear, descriptive, and unique?
  - Are `on` triggers appropriate for the workflow's purpose (e.g., `push`, `pull_request`, `workflow_dispatch`, `schedule`)? Are path/branch filters used effectively?
  - Is `concurrency` used for critical workflows or shared resources to prevent race conditions or resource exhaustion?
  - Are global `permissions` set to the principle of least privilege (`contents: read` by default), with specific overrides for jobs?
  - Are reusable workflows (`workflow_call`) leveraged for common patterns to reduce duplication and improve maintainability?
  - Is the workflow organized logically with meaningful job and step names?

- [ ] **Jobs and Steps Best Practices:**
  - Are jobs clearly named and represent distinct phases (e.g., `build`, `lint`, `test`, `deploy`)?
  - Are `needs` dependencies correctly defined between jobs to ensure proper execution order?
  - Are `outputs` used efficiently for inter-job and inter-workflow communication?
  - Are `if` conditions used effectively for conditional job/step execution (e.g., environment-specific deployments, branch-specific actions)?
  - Are all `uses` actions securely versioned (pinned to a full commit SHA)? Avoid `main` or `latest` tags.
  - Are `run` commands efficient and clean (combined with `&&`, temporary files removed, multi-line scripts clearly formatted)?
  - Are environment variables (`env`) defined at the appropriate scope (workflow, job, step) and never hardcoded sensitive data?
  - Is `timeout-minutes` set for long-running jobs to prevent hung workflows?

- [ ] **Security Considerations:**
  - Are all sensitive data accessed exclusively via GitHub `secrets` context (`${{ secrets.MY_SECRET }}`)? Never hardcoded, never exposed in logs (even if masked).
  - Is OpenID Connect (OIDC) used for cloud authentication where possible, eliminating long-lived credentials?
  - Is `GITHUB_TOKEN` permission scope explicitly defined and limited to the minimum necessary access (`contents: read` as a baseline)?
  - Are Software Composition Analysis (SCA) tools (e.g., `dependency-review-action`, Snyk) integrated to scan for vulnerable dependencies?
  - Are Static Application Security Testing (SAST) tools (e.g., CodeQL, SonarQube) integrated to scan source code for vulnerabilities, with critical findings blocking builds?
  - Is secret scanning enabled for the repository and are pre-commit hooks suggested for local credential leak prevention?
  - Is there a strategy for container image signing (e.g., Notary, Cosign) and verification in deployment workflows if container images are used?
  - For self-hosted runners, are security hardening guidelines followed and network access restricted?

- [ ] **Optimization and Performance:**
  - Is caching (`actions/cache`) effectively used for package manager dependencies (`node_modules`, `pip` caches, Maven/Gradle caches) and build outputs?
  - Are cache `key` and `restore-keys` designed for optimal cache hit rates (e.g., using `hashFiles`)?
  - Is `strategy.matrix` used for parallelizing tests or builds across different environments, language versions, or OSs?
  - Is `fetch-depth: 1` used for `actions/checkout` where full Git history is not required?
  - Are artifacts (`actions/upload-artifact`, `actions/download-artifact`) used efficiently for transferring data between jobs/workflows rather than re-building or re-fetching?
  - Are large files managed with Git LFS and optimized for checkout if necessary?

- [ ] **Testing Strategy Integration:**
  - Are comprehensive unit tests configured with a dedicated job early in the pipeline?
  - Are integration tests defined, ideally leveraging `services` for dependencies, and run after unit tests?
  - Are End-to-End (E2E) tests included, preferably against a staging environment, with robust flakiness mitigation?
  - Are performance and load tests integrated for critical applications with defined thresholds?
  - Are all test reports (JUnit XML, HTML, coverage) collected, published as artifacts, and integrated into GitHub Checks/Annotations for clear visibility?
  - Is code coverage tracked and enforced with a minimum threshold?

- [ ] **Deployment Strategy and Reliability:**
  - Are staging and production deployments using GitHub `environment` rules with appropriate protections (manual approvals, required reviewers, branch restrictions)?
  - Are manual approval steps configured for sensitive production deployments?
  - Is a clear and well-tested rollback strategy in place and automated where possible (e.g., `kubectl rollout undo`, reverting to previous stable image)?
  - Are chosen deployment types (e.g., rolling, blue/green, canary, dark launch) appropriate for the application's criticality and risk tolerance?
  - Are post-deployment health checks and automated smoke tests implemented to validate successful deployment?
  - Is the workflow resilient to temporary failures (e.g., retries for flaky network operations)?

- [ ] **Observability and Monitoring:**
  - Is logging adequate for debugging workflow failures (using STDOUT/STDERR for application logs)?
  - Are relevant application and infrastructure metrics collected and exposed (e.g., Prometheus metrics)?
  - Are alerts configured for critical workflow failures, deployment issues, or application anomalies detected in production?
  - Is distributed tracing (e.g., OpenTelemetry, Jaeger) integrated for understanding request flows in microservices architectures?
  - Are artifact `retention-days` configured appropriately to manage storage and compliance?

## Troubleshooting Common GitHub Workflows Issues (Deep Dive)

This section provides an expanded guide to diagnosing and resolving frequent problems encountered when working with GitHub Workflows.

### 1. Workflow Not Triggering or Jobs/Steps Skipping Unexpectedly

- **Root Causes:** Mismatched `on` triggers, incorrect `paths` or `branches` filters, erroneous `if` conditions, or `concurrency` limitations.
- **Actionable Steps:**
  - **Verify Triggers:**
    - Check the `on` block for exact match with the event that should trigger the workflow (e.g., `push`, `pull_request`, `workflow_dispatch`, `schedule`).
    - Ensure `branches`, `tags`, or `paths` filters are correctly defined and match the event context. Remember that `paths-ignore` and `branches-ignore` take precedence.
    - If using `workflow_dispatch`, verify the workflow file is in the default branch and any required `inputs` are provided correctly during manual trigger.
  - **Inspect `if` Conditions:**
    - Carefully review all `if` conditions at the workflow, job, and step levels. A single false condition can prevent execution.
    - Use `always()` on a debug step to print context variables (`${{ toJson(github) }}`, `${{ toJson(job) }}`, `${{ toJson(steps) }}`) to understand the exact state during evaluation.
    - Test complex `if` conditions in a simplified workflow.
  - **Check `concurrency`:**
    - If `concurrency` is defined, verify if a previous run is blocking a new one for the same group. Check the "Concurrency" tab in the workflow run.
  - **Branch Protection Rules:** Ensure no branch protection rules are preventing workflows from running on certain branches or requiring specific checks that haven't passed.

### 2. Permissions Errors (`Resource not accessible by integration`, `Permission denied`)

- **Root Causes:** `GITHUB_TOKEN` lacking necessary permissions, incorrect environment secrets access, or insufficient permissions for external actions.
- **Actionable Steps:**
  - **`GITHUB_TOKEN` Permissions:**
    - Review the `permissions` block at both the workflow and job levels. Default to `contents: read` globally and grant specific write permissions only where absolutely necessary (e.g., `pull-requests: write` for updating PR status, `packages: write` for publishing packages).
    - Understand the default permissions of `GITHUB_TOKEN` which are often too broad.
  - **Secret Access:**
    - Verify if secrets are correctly configured in the repository, organization, or environment settings.
    - Ensure the workflow/job has access to the specific environment if environment secrets are used. Check if any manual approvals are pending for the environment.
    - Confirm the secret name matches exactly (`secrets.MY_API_KEY`).
  - **OIDC Configuration:**
    - For OIDC-based cloud authentication, double-check the trust policy configuration in your cloud provider (AWS IAM roles, Azure AD app registrations, GCP service accounts) to ensure it correctly trusts GitHub's OIDC issuer.
    - Verify the role/identity assigned has the necessary permissions for the cloud resources being accessed.

### 3. Caching Issues (`Cache not found`, `Cache miss`, `Cache creation failed`)

- **Root Causes:** Incorrect cache key logic, `path` mismatch, cache size limits, or frequent cache invalidation.
- **Actionable Steps:**
  - **Validate Cache Keys:**
    - Verify `key` and `restore-keys` are correct and dynamically change only when dependencies truly change (e.g., `key: ${{ runner.os }}-node-${{ hashFiles('**/package-lock.json') }}`). A cache key that is too dynamic will always result in a miss.
    - Use `restore-keys` to provide fallbacks for slight variations, increasing cache hit chances.
  - **Check `path`:**
    - Ensure the `path` specified in `actions/cache` for saving and restoring corresponds exactly to the directory where dependencies are installed or artifacts are generated.
    - Verify the existence of the `path` before caching.
  - **Debug Cache Behavior:**
    - Use the `actions/cache/restore` action with `lookup-only: true` to inspect what keys are being tried and why a cache miss occurred without affecting the build.
    - Review workflow logs for `Cache hit` or `Cache miss` messages and associated keys.
  - **Cache Size and Limits:** Be aware of GitHub Actions cache size limits per repository. If caches are very large, they might be evicted frequently.

### 4. Long Running Workflows or Timeouts

- **Root Causes:** Inefficient steps, lack of parallelism, large dependencies, unoptimized Docker image builds, or resource bottlenecks on runners.
- **Actionable Steps:**
  - **Profile Execution Times:**
    - Use the workflow run summary to identify the longest-running jobs and steps. This is your primary tool for optimization.
  - **Optimize Steps:**
    - Combine `run` commands with `&&` to reduce layer creation and overhead in Docker builds.
    - Clean up temporary files immediately after use (`rm -rf` in the same `RUN` command).
    - Install only necessary dependencies.
  - **Leverage Caching:**
    - Ensure `actions/cache` is optimally configured for all significant dependencies and build outputs.
  - **Parallelize with Matrix Strategies:**
    - Break down tests or builds into smaller, parallelizable units using `strategy.matrix` to run them concurrently.
  - **Choose Appropriate Runners:**
    - Review `runs-on`. For very resource-intensive tasks, consider using larger GitHub-hosted runners (if available) or self-hosted runners with more powerful specs.
  - **Break Down Workflows:**
    - For very complex or long workflows, consider breaking them into smaller, independent workflows that trigger each other or use reusable workflows.

### 5. Flaky Tests in CI (`Random failures`, `Passes locally, fails in CI`)

- **Root Causes:** Non-deterministic tests, race conditions, environmental inconsistencies between local and CI, reliance on external services, or poor test isolation.
- **Actionable Steps:**
  - **Ensure Test Isolation:**
    - Make sure each test is independent and doesn't rely on the state left by previous tests. Clean up resources (e.g., database entries) after each test or test suite.
  - **Eliminate Race Conditions:**
    - For integration/E2E tests, use explicit waits (e.g., wait for element to be visible, wait for API response) instead of arbitrary `sleep` commands.
    - Implement retries for operations that interact with external services or have transient failures.
  - **Standardize Environments:**
    - Ensure the CI environment (Node.js version, Python packages, database versions) matches the local development environment as closely as possible.
    - Use Docker `services` for consistent test dependencies.
  - **Robust Selectors (E2E):**
    - Use stable, unique selectors in E2E tests (e.g., `data-testid` attributes) instead of brittle CSS classes or XPath.
  - **Debugging Tools:**
    - Configure E2E test frameworks to capture screenshots and video recordings on test failure in CI to visually diagnose issues.
  - **Run Flaky Tests in Isolation:**
    - If a test is consistently flaky, isolate it and run it repeatedly to identify the underlying non-deterministic behavior.

### 6. Deployment Failures (Application Not Working After Deploy)

- **Root Causes:** Configuration drift, environmental differences, missing runtime dependencies, application errors, or network issues post-deployment.
- **Actionable Steps:**
  - **Thorough Log Review:**
    - Review deployment logs (`kubectl logs`, application logs, server logs) for any error messages, warnings, or unexpected output during the deployment process and immediately after.
  - **Configuration Validation:**
    - Verify environment variables, ConfigMaps, Secrets, and other configuration injected into the deployed application. Ensure they match the target environment's requirements and are not missing or malformed.
    - Use pre-deployment checks to validate configuration.
  - **Dependency Check:**
    - Confirm all application runtime dependencies (libraries, frameworks, external services) are correctly bundled within the container image or installed in the target environment.
  - **Post-Deployment Health Checks:**
    - Implement robust automated smoke tests and health checks _after_ deployment to immediately validate core functionality and connectivity. Trigger rollbacks if these fail.
  - **Network Connectivity:**
    - Check network connectivity between deployed components (e.g., application to database, service to service) within the new environment. Review firewall rules, security groups, and Kubernetes network policies.
  - **Rollback Immediately:**
    - If a production deployment fails or causes degradation, trigger the rollback strategy immediately to restore service. Diagnose the issue in a non-production environment.

## Conclusion

These guidelines combine industry best practices for GitHub Workflows with Radius-specific patterns that prioritize testability, security, and developer experience. By following these principles, you'll create workflows that are:

- **Testable:** Core logic runs locally via Make, workflows can be tested on forks
- **Secure:** Least privilege, secret management, OIDC, dependency scanning
- **Efficient:** Caching, parallelization, shallow clones, smart artifact usage
- **Maintainable:** DRY principle, reusable workflows, clear naming conventions
- **Reliable:** Comprehensive testing, environment protection, rollback strategies

Remember: Workflows are code. Apply the same rigor to workflow development as you would to application code.

---

<!-- End of GitHub Workflows Best Practices -->
