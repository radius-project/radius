# Migration Plan: `rad env list` to Use Radius.Core/environments

## Summary

Migrate `rad env list` to list `Radius.Core/environments` (currently only done via `--preview` flag),
and remove the old `Applications.Core/environments` implementation.

---

## Files to DELETE

These files are exclusively for the old `Applications.Core` `rad env list` implementation
and have no shared usage with other commands:

1. **`pkg/cli/cmd/env/list/list.go`** — Old runner using `ApplicationsManagementClient.ListEnvironments`
2. **`pkg/cli/cmd/env/list/list_test.go`** — Tests for old runner

---

## Files to MODIFY

### 1. `pkg/cli/cmd/env/list/preview/list.go`

**Change:** Move this file to `pkg/cli/cmd/env/list/list.go` (replacing the deleted one), OR
update its package name and imports so it becomes the canonical implementation.

Concretely, the recommended approach:
- **Delete** `pkg/cli/cmd/env/list/list.go` (old)
- **Move** `pkg/cli/cmd/env/list/preview/list.go` → `pkg/cli/cmd/env/list/list.go`
- **Update** package declaration from `package preview` to `package list`
- **Remove** preview-specific doc strings ("preview", "Use the Radius.Core preview API surface")
- **Update** `Short` and `Long` descriptions to remove "(preview)" wording:
  - `Short: "List environments"`
  - `Long:  "List environments using the current, or specified workspace."`
- **Remove** the `Example` field's `rad env list` (or keep it as-is, it's fine)

### 2. `pkg/cli/cmd/env/list/preview/list_test.go`

**Change:** Move to `pkg/cli/cmd/env/list/list_test.go` (replacing the deleted one)
- Update package declaration from `package preview` to `package list`
- Keep all test logic — it tests the `Radius.Core` implementation which is the new canonical behavior

### 3. `cmd/rad/cmd/root.go`

**Lines to change (~50-51, ~358-361):**

Remove:
```go
env_list "github.com/radius-project/radius/pkg/cli/cmd/env/list"
env_list_preview "github.com/radius-project/radius/pkg/cli/cmd/env/list/preview"
```
Add:
```go
env_list "github.com/radius-project/radius/pkg/cli/cmd/env/list"
```
(Keep only the single `env_list` import pointing to the updated package)

Remove:
```go
envListCmd, _ := env_list.NewCommand(framework)
previewListCmd, _ := env_list_preview.NewCommand(framework)
wirePreviewSubcommand(envListCmd, previewListCmd)
envCmd.AddCommand(envListCmd)
```
Replace with:
```go
envListCmd, _ := env_list.NewCommand(framework)
envCmd.AddCommand(envListCmd)
```

**Note:** Do NOT remove `wirePreviewSubcommand` itself — it is still used by other commands
(env create, env delete, env show, env switch).

---

## Files to DELETE (preview directory, after move)

4. **`pkg/cli/cmd/env/list/preview/list.go`** — after its content is moved to `pkg/cli/cmd/env/list/list.go`
5. **`pkg/cli/cmd/env/list/preview/list_test.go`** — after its content is moved to `pkg/cli/cmd/env/list/list_test.go`

The `pkg/cli/cmd/env/list/preview/` directory will be empty and can be removed.

---

## Shared Code — DO NOT DELETE

The following code is used by other commands and must be preserved:

- **`pkg/cli/clients/clients.go`** — `ListEnvironments` and `ListEnvironmentsAll` interface methods
  - `ListEnvironmentsAll` is used by `pkg/cli/cmd/uninstall/kubernetes/kubernetes.go` and `pkg/cli/cmd/radinit/environment.go`
  - `ListEnvironments` is currently only used by the old list command, but removing it from the interface requires updating the mock too (see below)

- **`pkg/cli/clients/management.go`** — `ListEnvironments` and `ListEnvironmentsAll` implementations
  - Same reasoning: `ListEnvironmentsAll` is used elsewhere

- **`pkg/cli/clients/mock_applicationsclient.go`** — Auto-generated mock
  - If `ListEnvironments` is removed from the interface, regenerate the mock (`go generate ./...` or equivalent)
  - Alternatively, leave `ListEnvironments` in the interface for now (no harm, minor dead code)
  - **RECOMMENDATION:** Remove `ListEnvironments` (not `ListEnvironmentsAll`) from the interface in `clients.go` and regenerate the mock, since it will no longer be called by anything after this migration.

- **`cmd/rad/cmd/root.go`** — `wirePreviewSubcommand` function
  - Still used by: env create, env delete, env show, env switch
  - Only the env list wiring should be removed

---

## Summary of Changes

| File | Action |
|------|--------|
| `pkg/cli/cmd/env/list/list.go` | DELETE (old Applications.Core implementation) |
| `pkg/cli/cmd/env/list/list_test.go` | DELETE (old tests) |
| `pkg/cli/cmd/env/list/preview/list.go` | MOVE → `pkg/cli/cmd/env/list/list.go`, update package + descriptions |
| `pkg/cli/cmd/env/list/preview/list_test.go` | MOVE → `pkg/cli/cmd/env/list/list_test.go`, update package |
| `cmd/rad/cmd/root.go` | Remove `env_list_preview` import and `wirePreviewSubcommand` wiring for list |
| `pkg/cli/clients/clients.go` | Remove `ListEnvironments` (not `ListEnvironmentsAll`) from interface |
| `pkg/cli/clients/management.go` | Remove `ListEnvironments` implementation (not `ListEnvironmentsAll`) |
| `pkg/cli/clients/mock_applicationsclient.go` | Regenerate or manually remove `ListEnvironments` mock methods |
| `pkg/cli/clients/management_test.go` | Remove `TestListEnvironments` test case (keep `TestListEnvironmentsAll`) |

---

## Verification Steps After Migration

1. `go build ./cmd/rad/...` — ensure the binary compiles
2. `go test ./pkg/cli/cmd/env/list/...` — new tests pass
3. `go test ./pkg/cli/clients/...` — client tests pass
4. `rad env list` runs and lists `Radius.Core/environments`
5. `rad env list --preview` should no longer exist (verify with `rad env list --help`)
