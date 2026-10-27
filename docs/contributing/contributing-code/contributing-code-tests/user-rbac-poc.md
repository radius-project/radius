# Testing the user RBAC prototype

## Purpose

Test read-only access, environment restrictions, custom roles, revocation, and administrator protection from the [RBAC feature spec](../../../../eng/design-notes/security/2026-09-built-in-rbac-feature-spec.md#key-scenarios).

Both user and internal RBAC are enforced. Internal RBAC uses prototype snapshot `093010a90`. Use only a disposable local cluster: deployment-engine identity uses pod IP, and component read/shared-store isolation is incomplete.

## Prerequisites

- This checkout, Go, GNU Make, Git, kind, kubectl, Helm, jq, Bicep, and curl (setup only). Install Bicep with `make install-bicep` and add `$HOME/.local/bin` to `PATH`.
- Running Podman or Docker, enough memory for Kubernetes and Radius, and internet access.
- Run commands from the repository root. Do not share `dist/`; it contains cluster credentials.

### Get the prototype

Install the tools listed above using the [contributor prerequisites](../contributing-code-prerequisites/README.md). The checkout commands below also require the [GitHub CLI](https://cli.github.com/).

For a fresh checkout:

```sh
git clone https://github.com/radius-project/radius.git
cd radius
gh pr checkout 13480
```

In an existing checkout, use a clean working directory and run `gh pr checkout 13480`. If you are already in the prepared POC working session, keep that checkout.

Install Bicep if needed, then select your running container runtime:

```sh
make install-bicep
export PATH="$HOME/.local/bin:$PATH"
export RBAC_POC_RUNTIME=podman
podman info
```

For Docker, set `RBAC_POC_RUNTIME=docker` and check `docker info` instead. On macOS with Podman, start your existing VM with `podman machine start` if it is stopped. Continue with Step 1 once the runtime responds.

## Steps

Run commands individually. Expected denials must report `AuthorizationFailed`, not connection or configuration errors.

### 1. Prepare the cluster

**Already set up? Skip to Step 2.**

For a new kit:

```sh
bash hack/user-rbac/poc.sh up
bash hack/user-rbac/poc.sh test
```

`up` builds and installs Radius on the dedicated kind cluster. It grants authenticated test users entry to the Radius API, not access to Radius resources. It also gives `poc-admin` the Kubernetes permission Radius checks before allowing initial administrator bootstrap.

`test` initializes Radius authorization, creates the bootstrap administrator, grants permanent installation-wide administration to `poc-admins`, and creates the test fixtures. It runs API, CLI, and Bicep scenarios and leaves enforcement enabled. The identities and broad API-entry binding are test setup; the bootstrap itself uses Radius's permission-checked initialization API. Normal `rad install` does not yet perform this user-RBAC bootstrap.

For an existing, initialized kit, repeat setup with:

```sh
RADIUS_USER_RBAC_RESUME=true bash hack/user-rbac/poc.sh test
```

Resume skips initialization and the initial Disabled/DryRun scenarios; it repeats the remaining scenarios against the existing policy.

**Expected:** tests pass and leave `cli-demo`, staging/production environments, and these test workspaces:

| Workspace       | Access                                                                   |
|-----------------|--------------------------------------------------------------------------|
| `poc-admin`     | Radius administrator                                                     |
| `poc-developer` | Manage team applications; read both environments; deploy only to staging |
| `poc-reader`    | Read `cli-demo` through its group assignment                             |
| `poc-outsider`  | No application access                                                    |

These are impersonated test identities, not separate logins. Setup creates the fixtures; the following steps run `rad` directly.

### 2. Set the CLI paths

Use the same terminal for all test commands:

```sh
POC_STATE="$PWD/dist/user-rbac-poc"
RAD="$PWD/dist/$(go env GOOS)_$(go env GOARCH)/release/rad"
RAD_CONFIG="$POC_STATE/cli/rad.json"
RAD_HOME="$POC_STATE/cli/home"
export BICEP="$(command -v bicep)"
```

For a named kit, replace `dist/user-rbac-poc` with `dist/$RBAC_POC_CLUSTER`. Keep `HOME="$RAD_HOME"` on each command: it selects the test kubeconfig without changing your normal configuration.

`--config "$RAD_CONFIG"` selects the isolated test workspaces; `--workspace` selects the test identity and target defaults. These flags are not new RBAC requirements. A normally configured Radius workspace does not need a separate config file.

The wrapper supplies the test configuration, workspace, and `HOME` for you. For example, this is equivalent to the direct staging deployment in Step 6:

```sh
bash hack/user-rbac/poc.sh rad poc-developer deploy "$POC_STATE/cli/application.bicep"
```

### 3. Read and list applications

```sh
HOME="$RAD_HOME" "$RAD" app show cli-demo --workspace poc-reader --config "$RAD_CONFIG" -o json
HOME="$RAD_HOME" "$RAD" app list --workspace poc-reader --config "$RAD_CONFIG" -o json
HOME="$RAD_HOME" "$RAD" app list --workspace poc-outsider --config "$RAD_CONFIG" -o json
```

**Expected:** the reader can read and list `cli-demo`. The outsider list returns `null`.

```sh
HOME="$RAD_HOME" "$RAD" app show cli-demo --workspace poc-outsider --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" app show cli-does-not-exist --workspace poc-outsider --config "$RAD_CONFIG"
```

**Expected:** both fail with `AuthorizationFailed`, without revealing whether the application exists.

### 4. Try unauthorized changes

Do not change the workspace on these mutation commands to admin or developer.

```sh
HOME="$RAD_HOME" "$RAD" resource create Radius.Core/applications cli-demo \
  -f "$POC_STATE/cli/application.json" --workspace poc-reader --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" resource delete Radius.Core/applications cli-demo --yes \
  --workspace poc-reader --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" app show cli-demo --group rbac-poc-platform \
  --workspace poc-developer --config "$RAD_CONFIG"
```

**Expected:** all fail with `AuthorizationFailed`: application write, delete, and cross-group read respectively.

```sh
HOME="$RAD_HOME" "$RAD" app show cli-demo --workspace poc-admin --config "$RAD_CONFIG" -o json
```

**Expected:** the application still exists, is `Succeeded`, and has no `must-not-apply` tag. `resource create` would update the existing application if authorized; `--yes` skips confirmation, not authorization.

### 5. Read an environment without managing it

```sh
HOME="$RAD_HOME" "$RAD" env show cli-production --group rbac-poc-platform \
  --workspace poc-developer --config "$RAD_CONFIG" -o json
HOME="$RAD_HOME" "$RAD" resource delete Radius.Core/environments cli-production --yes \
  --group rbac-poc-platform --workspace poc-developer --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" env show cli-production --group rbac-poc-platform \
  --workspace poc-admin --config "$RAD_CONFIG" -o json
```

**Expected:** both reads succeed. Delete fails with `AuthorizationFailed` for `Radius.Core/environments/delete`.

### 6. Deploy to staging, not production

This deploys the kit's [application Bicep template](../../../../test/user-rbac/templates/application.bicep), which updates application metadata, not Radius itself or a sample workload. The CLI compiles Bicep and supplies the `environment` parameter from the workspace or `--environment`. The `application.json` used in Step 4 is only a request body for `rad resource create`, not a deployment template.

```sh
HOME="$RAD_HOME" "$RAD" deploy "$POC_STATE/cli/application.bicep" \
  --workspace poc-developer --config "$RAD_CONFIG"
```

**Expected:** `Deployment Complete`. Workspace defaults select the team resource group and staging environment.

This local build prints `WARNING: Radius type compatibility is not verified`; the warning can include HTTP `403` from the version endpoint. The kit uses a locally packaged extension and an edge CLI rather than matching published releases. Judge the deployment by its final result; a final `AuthorizationFailed` is still a denial.

```sh
HOME="$RAD_HOME" "$RAD" deploy "$POC_STATE/cli/application.bicep" \
  --parameters applicationName=cli-must-not-exist \
  --workspace poc-developer --config "$RAD_CONFIG" --group rbac-poc-team \
  --environment /planes/radius/local/resourceGroups/rbac-poc-platform/providers/Radius.Core/environments/cli-production
HOME="$RAD_HOME" "$RAD" app show cli-must-not-exist \
  --workspace poc-admin --config "$RAD_CONFIG" -o json
```

**Expected:** deployment fails with `AuthorizationFailed` for `Radius.Core/environments/use/action`. Admin lookup reports that the application was not found: no application was created.

### 7. Check identity and enforcement

```sh
TEAM=/planes/radius/local/resourceGroups/rbac-poc-team
CLI_APP=/planes/radius/local/resourceGroups/rbac-poc-team/providers/Radius.Core/applications/cli-demo
CLI_ENV=/planes/radius/local/resourceGroups/rbac-poc-platform/providers/Radius.Core/environments
HOME="$RAD_HOME" "$RAD" auth whoami --workspace poc-developer --config "$RAD_CONFIG" -o json
HOME="$RAD_HOME" "$RAD" auth status --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth enforcement show --workspace poc-admin --config "$RAD_CONFIG"
```

**Expected:** developer identity `poc-developer`, group `poc-developers`; admin status is `true`; mode is `Enforce`.

Inspect roles and assignments as the Radius administrator:

```sh
HOME="$RAD_HOME" "$RAD" auth role list --scope / --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth assignment list --scope / --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth role list --scope "$TEAM" --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth assignment list --scope "$TEAM" --include-inherited \
  --workspace poc-admin --config "$RAD_CONFIG"
```

**Expected:** installation assignments include the permanent `poc-admins` administrator grant. `--include-inherited` includes ancestor grants; listing at `/` alone does not recursively list descendant assignments. Kubernetes `cluster-admin` and Radius administrator are separate permissions.

### 8. Check identity and permissions

```sh
HOME="$RAD_HOME" "$RAD" auth permission show Radius.Core/environments/use/action \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth access check --scope "$CLI_APP" \
  --action Radius.Core/applications/write --workspace poc-developer --config "$RAD_CONFIG" -o json
HOME="$RAD_HOME" "$RAD" auth access check --scope "$CLI_ENV/cli-staging" \
  --action Radius.Core/environments/use/action --workspace poc-developer --config "$RAD_CONFIG" -o json
HOME="$RAD_HOME" "$RAD" auth access check --scope "$CLI_ENV/cli-production" \
  --action Radius.Core/environments/use/action --action Radius.Core/environments/delete \
  --workspace poc-developer --config "$RAD_CONFIG" -o json
HOME="$RAD_HOME" "$RAD" auth access explain --scope "$CLI_APP" \
  --action Radius.Core/applications/read --workspace poc-reader --config "$RAD_CONFIG" -o json
```

**Expected:** write and staging use are allowed; production use and deletion are denied. The reader explanation allows read but hides grant details the reader cannot inspect. These checks do not change resources.

### 9. Grant a custom read-only role

This temporarily gives the outsider access to `cli-demo`. Complete Step 10 to remove it.

```sh
PM_ROLE=/providers/Radius.Core/roleDefinitions/pm-walkthrough-reader
PM_GRANT="$CLI_APP/providers/Radius.Core/roleAssignments/pm-walkthrough-reader"
ISSUER=$(HOME="$RAD_HOME" "$RAD" auth whoami --workspace poc-admin --config "$RAD_CONFIG" -o json | jq -r .issuer)
HOME="$RAD_HOME" "$RAD" auth role create pm-walkthrough-reader --scope / \
  --permission Radius.Core/applications/read --assignable-scope "$TEAM" --yes \
  --workspace poc-developer --config "$RAD_CONFIG"
```

**Expected:** `AuthorizationFailed`. Developers cannot create roles.

Create the same role as admin and assign it to the outsider at the application scope:

```sh
HOME="$RAD_HOME" "$RAD" auth role create pm-walkthrough-reader --scope / \
  --permission Radius.Core/applications/read --assignable-scope "$TEAM" --yes \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth role update "$PM_ROLE" --description "Temporary PM test" --yes \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth assignment create pm-walkthrough-reader --scope "$CLI_APP" \
  --principal user:poc-outsider --issuer "$ISSUER" --role "$PM_ROLE" --yes \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth assignment list --scope "$CLI_APP" --include-inherited \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth access list --scope "$CLI_APP" \
  --principal group:poc-readers --issuer "$ISSUER" \
  --workspace poc-admin --config "$RAD_CONFIG" -o json
```

**Expected:** the role and assignment are created; the assignment summary names the outsider and exact application scope. Effective access for the readers group includes application read.

```sh
HOME="$RAD_HOME" "$RAD" app show cli-demo --workspace poc-outsider --config "$RAD_CONFIG" -o json
HOME="$RAD_HOME" "$RAD" app list --workspace poc-outsider --config "$RAD_CONFIG" -o json
HOME="$RAD_HOME" "$RAD" resource create Radius.Core/applications cli-demo \
  -f "$POC_STATE/cli/application.json" --workspace poc-outsider --config "$RAD_CONFIG"
```

**Expected:** read succeeds and the list includes `cli-demo`. Write still fails with `AuthorizationFailed`.

### 10. Revoke the grant

Delete the assignment before its role:

```sh
HOME="$RAD_HOME" "$RAD" auth assignment delete "$PM_GRANT" --yes \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth role delete "$PM_ROLE" --yes \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" app show cli-demo --workspace poc-outsider --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" app list --workspace poc-outsider --config "$RAD_CONFIG" -o json
```

**Expected:** deletes succeed; read fails with `AuthorizationFailed`; list returns `null`. If interrupted, rerun these cleanup commands; `NotFound` is acceptable only for an already deleted role or assignment.

### 11. Deploy a role and assignment from Bicep

The test kit prepares `cli/access.bicep` and its local extension. Run this command and accept the default **No**:

```sh
HOME="$RAD_HOME" "$RAD" deploy "$POC_STATE/cli/access.bicep" \
  --parameters identityIssuer="$ISSUER" --parameters assignmentScope="$CLI_APP" \
  --workspace poc-admin --config "$RAD_CONFIG"
```

**Expected:** the summary shows `poc-outsider`, its issuer, the custom role, and the exact application scope. Cancelling changes nothing. Non-interactive use requires `--yes`.

Apply the same template:

```sh
HOME="$RAD_HOME" "$RAD" deploy "$POC_STATE/cli/access.bicep" --yes \
  --parameters identityIssuer="$ISSUER" --parameters assignmentScope="$CLI_APP" \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" app show cli-demo --workspace poc-outsider --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" deploy "$POC_STATE/cli/access.bicep" --yes \
  --parameters identityIssuer="$ISSUER" --parameters assignmentScope="$CLI_APP" \
  --workspace poc-developer --config "$RAD_CONFIG"
```

**Expected:** the admin deployment reports `Deployment Complete`; outsider read succeeds; the developer deployment fails with `AuthorizationFailed`. A policy-only deployment needs no environment, and `--yes` never bypasses authorization.

Delete the assignment and role explicitly; removing them from the template does not revoke access:

```sh
TEMPLATE_ROLE="$TEAM/providers/Radius.Core/roleDefinitions/poc-template-reader"
TEMPLATE_GRANT=$(HOME="$RAD_HOME" "$RAD" auth assignment list --scope "$CLI_APP" \
  --workspace poc-admin --config "$RAD_CONFIG" -o json |
  jq -er '.[] | select(.properties.roleDefinitionId | endswith("/poc-template-reader")) | .id')
HOME="$RAD_HOME" "$RAD" auth assignment delete "$TEMPLATE_GRANT" --yes \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth role delete "$TEMPLATE_ROLE" --yes \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" app show cli-demo --workspace poc-outsider --config "$RAD_CONFIG"
```

**Expected:** deletes succeed; outsider read fails with `AuthorizationFailed`.

### 12. Inspect access changes

```sh
HOME="$RAD_HOME" "$RAD" auth history list --workspace poc-admin --config "$RAD_CONFIG" -o json |
  jq '.[] | select(.properties.policyChange.target | contains("pm-walkthrough-reader"))'
```

**Expected:** creation and deletion events from Steps 9–10, including principal, target, request ID, and generation. Denied workload calls are not policy changes and do not appear here.

### 13. Check administrator protection

Use only the original kit policy. If you added another permanent administrator, skip the delete: it may be allowed.

```sh
HOME="$RAD_HOME" "$RAD" auth assignment delete permanent-admins --scope / --yes \
  --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" app show cli-demo --workspace poc-admin --config "$RAD_CONFIG" -o json
```

**Expected:** deletion fails with `LastAdministratorProtected`; admin read succeeds.

### 14. Preview enforcement, then restore it

```sh
HOME="$RAD_HOME" "$RAD" auth enforcement preview --yes --workspace poc-admin --config "$RAD_CONFIG"
HOME="$RAD_HOME" "$RAD" auth enforcement enable --yes --workspace poc-admin --config "$RAD_CONFIG"
```

**Expected:** mode changes to `DryRun`, then back to `Enforce`. Do not run denial tests while in `DryRun`: workload denials are logged rather than blocked. `auth enforcement set --mode Disabled` is rejected after adoption; rollback to `DryRun` is supported.

### 15. Finish

Keep the cluster for more testing, or delete only this kit:

```sh
bash hack/user-rbac/poc.sh down
```

## Verification

Record pass/fail for each step and include the error and `TraceId` for failures. Never include the private kubeconfig.

This walkthrough covers parts of feature-spec scenarios 1, 2, 3, and 5, including real Bicep policy deployments. The automated tests also cover resource-group grants, updates, incremental omission, and valid/invalid mixed policy-workload templates. An invalid policy plan changes nothing. Valid policy changes commit together before workloads run; a later workload failure does not roll back policy.

Preflight supports up to 100 top-level Radius resources, parameters, variables, deterministic naming, and references to declared resources. Modules, loops, conditions, and external-state expressions are rejected before writes. Templates containing policy must declare the workloads and recipe packs they create: the CLI does not create an implicit application or default recipe pack before policy validation.

GitOps/automation identities, dynamic resource types, production recovery, and full cross-client behavior remain outside this walkthrough. Inspecting another user evaluates direct grants only; that user's group membership is unavailable. History is incomplete and lists at most 100 generations.

The automated API and CLI tests are in [test/user-rbac/](../../../../test/user-rbac/). Logs remain under `dist/user-rbac-poc/scenarios.log`, `cli/commands.log`, and `ucp-audit.jsonl`.

## Troubleshooting

| Error                                     | Action                                                                                                           |
|-------------------------------------------|------------------------------------------------------------------------------------------------------------------|
| Missing Go or another prerequisite        | Add the required tool to `PATH` before Step 1.                                                                   |
| Cannot connect to Podman                  | Check `podman machine list`; start the existing VM with `podman machine start`.                                  |
| Cluster stopped after a VM restart        | Run `podman start radius-user-rbac-poc-control-plane` for this kit's existing node. Do not recreate the cluster. |
| Missing `~/.kube/config`                  | Check the `HOME="$RAD_HOME"` prefix and repeat Step 2.                                                           |
| Missing `cli/rad.json`                    | Complete Step 1.                                                                                                 |
| Initialization returns `409` during setup | The kit is already initialized. Use the resume command in Step 1.                                                |
| `AuthorizationIndeterminate` or `503`     | Not an expected permission denial. Run `bash hack/user-rbac/poc.sh logs` and inspect the UCP log.                |
