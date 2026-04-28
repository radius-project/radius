# Feature Specification: Replace Radius Workspaces with `rad configure --defaults`

## Background

### The workspace concept confuses users

Radius today exposes a CLI-only concept called a **workspace**, configured through commands like `rad workspace create`, `rad workspace switch`, `rad workspace list`, and `rad workspace delete`. Despite the API-style verbs, a workspace is **not** a Radius API object — it is purely a client-side bundle stored in `~/.rad/config.yaml` that combines:

- a Kubernetes connection (a kube context name),
- a default resource group (stored as a fully-qualified URI like `/planes/radius/local/resourceGroups/default`), and
- a default environment (also stored as a URI).

This naming and command shape causes recurring user confusion:

- **"Is a workspace an API resource?"** `rad workspace create` reads like `rad app create` or `rad env create`, which *do* create server-side objects. Users assume workspaces are similar and look for them in `rad resource list`, in the dashboard, or in their cluster's CRDs — where they do not exist.
- **"What is the difference between a workspace, a resource group, and an environment?"** New users see three concepts in the docs (`workspace`, `group`, `environment`) but only two of them (group, environment) actually correspond to anything on the cluster. The third is a CLI-local container whose only job is to remember which group and environment to use by default.
- **"Why do I need to switch workspaces to switch resource groups?"** Day-to-day, what users actually want is to change which resource group or environment their next `rad` commands target. Today that is spread across three commands (`rad workspace switch`, `rad group switch`, `rad env switch`) with subtly different semantics, plus per-command `--workspace`, `--group`, and `--environment` flags whose precedence is not obvious.
- **"Why does my workspace stop working when I switch clusters?"** A workspace pins a specific kube context. If the user switches clusters with `kubectl config use-context`, the workspace either silently uses the old (now-wrong) context or fails opaquely, depending on how the workspace was set up.

The net effect is a surface area whose primary job is to remember two strings (a default group name and a default environment name) per cluster, but which appears to users as a fourth top-level concept on equal footing with apps, environments, and resource groups.

### `az configure --defaults` as the model

The `az` CLI solves the same problem with a much smaller surface: `az configure --defaults group=my-rg location=westus2`. There is no client-side "workspace" object — `az` simply remembers a few key/value defaults and applies them to subsequent commands. Users discover the feature once, learn one command, and never have to reason about a CLI-only entity that mirrors API objects.

This refactor adopts the same model for Radius. Users set defaults with `rad configure --defaults group=<name> environment=<name>`, list them with `rad configure --list-defaults`, and clear individual keys by setting them to an empty value. The defaults are scoped to the active Kubernetes context (so switching clusters with `kubectl config use-context` automatically selects the right defaults), and Radius commands fall back through a clear, per-key precedence chain when a default is not set. New users never encounter the word "workspace"; existing users keep working without manual migration because the legacy `workspaces:` block is still read.

The "workspace" concept is thereby downgraded from a top-level CLI noun to an internal storage detail and a one-shot `-w` flag for users who already rely on named workspace bundles.

## Storage strategy

The existing `workspaces:` block in `~/.rad/config.yaml` is **never written** by new commands and is **read only as a fallback** when no `defaults:` entry matches the active Kubernetes context. New commands write a single new top-level block:

```yaml
defaults:
  my-kubecontext:
    group: my-group
    environment: my-env
  my-other-kubecontext:
    group: prod-group
    environment: prod-env
workspaces:        # read-only fallback (unchanged on disk by new commands)
  default: default
  items:
    default:
      connection: { context: my-kubecontext, kind: kubernetes }
      scope: /planes/radius/local/resourceGroups/default
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
```

**Resolution order** for the group/environment used by a `rad` command (highest precedence first):

1. **Per-command `-g/--group` / `-e/--environment` flag** on the individual command.
2. **Per-command `-w/--workspace <name>` flag**: if present, the named workspace from `workspaces.items.<name>` supplies group (from `scope`), environment (from `environment`), and Kubernetes connection (from `connection.context`) for keys not satisfied by step 1.
3. **`defaults.<active-kube-context>.<key>`** in the new `defaults:` block.
4. **`workspaces.default`** — the entry named by `workspaces.default` supplies group/environment/connection for any key still unresolved.
5. **Built-in fallback**: the literal name `default` is used for `group` and/or `environment` if still unresolved. Because `rad init` creates a resource group named `default` and an environment named `default`, this gives a working zero-config experience without `rad init` needing to write to `~/.rad/config.yaml` at all.
6. Error only if even the built-in `default` fallback is unusable (e.g., the user has explicitly cleared a default with `rad configure --defaults group=` and the literal `default` group does not exist on the cluster). The remediation message names `--group`/`--environment`, `--workspace`, and `rad configure --defaults`.

Resolution is **per key independently**: e.g., `--group` set on the command line, `environment` resolved from `defaults:`, and Kubernetes connection from `workspaces.default` is a valid combination. Steps stop at the first source that supplies each key.

The Kubernetes connection has no `default` literal fallback — it must come from `-w`, `workspaces.default`, or the active kube context (the active context is implicit and used by all sources in steps 3–5 unless `-w` or `workspaces.default` overrides it).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - First-time user runs `rad init` with no config file written for defaults (Priority: P1)

A new user installs the CLI, points `kubectl` at their cluster, and runs `rad init`. The CLI installs the Radius control plane and creates a resource group named `default` and an environment named `default` on the cluster. **It does not need to write a `defaults:` block to `~/.rad/config.yaml`** — subsequent `rad` commands resolve to the literal name `default` via the built-in fallback (FR-012 step 5). No "workspace" is created, named, or mentioned anywhere in user-visible output.

**Why this priority**: This is the entry point for every new user. The combination of "no workspace concept" and "no required config file write" is the defining shape of the new UX.

**Independent Test**: Run `rad init` on a fresh machine, then run `rad env show` with no flags. Confirm that the environment named `default` from the resource group named `default` is shown \u2014 verifying that `rad init` created both, that the literal-`default` fallback resolved both keys without any `defaults:` block being written, and that no user-facing output mentioned \"workspace\".

**Acceptance Scenarios**:

1. **Given** a fresh machine with no Radius config and active kube context `my-kubecontext`, **When** the user runs `rad init` and accepts the defaults, **Then** a resource group named `default` and an environment named `default` are created on the cluster, no `defaults:` block is written to `~/.rad/config.yaml` solely for this purpose, and the success message contains no occurrence of "workspace".
2. **Given** a successful `rad init`, **When** the user runs `rad app list` immediately afterward without flags, **Then** the command succeeds by resolving to group `default` and environment `default` via the built-in literal fallback.
3. **Given** a successful `rad init`, **When** the user runs `rad workspace list` or `rad env switch` or `rad group switch`, **Then** the CLI fails with a "command removed" error that names the `rad configure --defaults` replacement.
4. **Given** a successful `rad init` followed by `rad configure --defaults group=my-rg`, **When** the user runs `rad app list`, **Then** the command targets `my-rg` (the configured default takes precedence over the literal `default` fallback).

---

### User Story 2 - Set, list, and clear defaults with `rad configure` (Priority: P1)

A user manages defaults via `rad configure --defaults <key>=<value>` (set), `rad configure --defaults <key>=` (clear, empty value), and `rad configure --list-defaults` (inspect).

**Why this priority**: This is the primary replacement surface for `rad workspace switch`, `rad group switch`, and `rad env switch`. Users must be able to perform every workspace-default operation through `rad configure`.

**Independent Test**: From any valid Radius config, run `rad configure --defaults group=dev-rg`, `rad configure --defaults environment=dev-env`, then `rad configure --list-defaults`. Verify the values appear under the active kube context and that a scoped command uses them. Then run `rad configure --defaults group=` and verify the next scoped command falls through to the built-in literal `default` fallback (or fails with a clear remediation message if no resource group named `default` exists on the cluster).

**Acceptance Scenarios**:

1. **Given** an active kube context `my-kubecontext` and no existing defaults entry for it, **When** the user runs `rad configure --defaults group=my-rg`, **Then** the CLI validates that `my-rg` exists on the cluster connected via `my-kubecontext`, persists `defaults.my-kubecontext.group: my-rg` to `~/.rad/config.yaml`, and prints a confirmation including the kube context, key, and new value.
2. **Given** existing defaults for `my-kubecontext`, **When** the user runs `rad configure --defaults environment=my-env`, **Then** the CLI validates the environment within the configured default group and persists `defaults.my-kubecontext.environment: my-env`.
3. **Given** a config with defaults for one or more contexts, **When** the user runs `rad configure --list-defaults`, **Then** the CLI prints all `defaults:` entries grouped by kube context (with the active context indicated), and supports `--output json` returning a stable schema.
4. **Given** a default group is set for the active context, **When** the user runs `rad configure --defaults group=` (empty value), **Then** that single key is removed from `defaults.<active-context>`. Subsequent commands fall through the resolution order; if no `workspaces.default` supplies the key, they fall through to the literal `default` fallback. If the literal `default` group does not exist on the cluster, the command fails with the standard remediation message.
5. **Given** removing the last key for a context leaves an empty map, **When** the operation completes, **Then** the empty `defaults.<context>` entry is removed from the file (no empty `{}` left behind).
6. **Given** the user passes `rad configure --defaults group=<name>` for a group that does not exist on the connected cluster, **When** the command runs, **Then** the CLI fails with a clear error and does not modify the config file.
7. **Given** the user passes multiple key/value pairs, e.g. `rad configure --defaults group=my-rg environment=my-env`, **When** the command runs, **Then** the CLI validates and persists all of them atomically (all-or-nothing) under the active kube context.
8. **Given** the user passes an unknown key, e.g. `rad configure --defaults foo=bar`, **When** the command runs, **Then** the CLI fails with a clear error listing the supported keys (`group`, `environment`) and does not modify the file.

---

### User Story 3 - Per-context defaults make kube context switches automatic (Priority: P1)

A user configures defaults for two kube contexts (`dev-cluster` and `prod-cluster`). They switch between them with `kubectl config use-context` and run `rad` commands. Each `rad` invocation automatically picks up the right defaults for the active context — no warnings, no prompts, no `rad config set context` step.

**Why this priority**: This is the design's signature feature. The kube-context-keyed `defaults:` block replaces the static workspace-to-context binding and eliminates the mismatch class of problems entirely.

**Independent Test**: Create defaults for two contexts, switch contexts via `kubectl`, run scoped commands without flags, and verify each one targets the matching cluster's defaults.

**Acceptance Scenarios**:

1. **Given** `defaults.dev-cluster.group: dev-rg` and `defaults.prod-cluster.group: prod-rg`, and active context `dev-cluster`, **When** the user runs `rad app list`, **Then** the command targets `dev-cluster` with group `dev-rg`.
2. **Given** the same config, **When** the user runs `kubectl config use-context prod-cluster && rad app list`, **Then** the command targets `prod-cluster` with group `prod-rg` automatically — no warning or prompt.
3. **Given** active kube context `unknown-cluster` and no `defaults.unknown-cluster` entry, no `workspaces.default` resolving the missing keys, and resource groups/environments named `default` exist on `unknown-cluster`, **When** the user runs `rad app list`, **Then** the command succeeds via the literal `default` fallback. If those literal-`default` resources do not exist, the command fails with a remediation message naming `--group`, `--workspace`, and `rad configure --defaults`.
4. **Given** the user has no active kube context (e.g., `KUBECONFIG` empty or current-context unset) and no `--workspace` flag is passed, **When** the user runs any scoped `rad` command, **Then** the CLI fails fast with a remediation message pointing at `kubectl config use-context` and `rad init`. (If `--workspace <name>` is passed, that workspace's `connection.context` is used and the command proceeds.)

---

### User Story 4 - Per-command flags override; `--workspace` remains a one-shot selector (Priority: P1)

Every scoped command supports `-g/--group`, `-e/--environment`, and `-w/--workspace` flags. Per-command `-g`/`-e` flags always win. `-w <name>` selects a named workspace from the legacy `workspaces.items` block as a single unit (its `connection.context`, `scope`, and `environment` are used for any keys not already supplied by `-g`/`-e`). None of these flags mutate the config file.

**Why this priority**: Without consistent override semantics, the new model is worse than workspaces. `-w` is preserved so existing scripts and users who rely on named workspaces keep working without re-tooling.

**Independent Test**: With defaults set, run scoped commands without flags and confirm they target the defaults. Repeat with `-g` and `-e` and confirm they override without changing the config. Repeat with `-w <name>` against a config that contains a non-default named workspace and confirm the command targets that workspace's group, environment, and connection — still without mutating the file.

**Acceptance Scenarios**:

1. **Given** `defaults.my-kubecontext.group: dev-rg, environment: dev-env` and active context `my-kubecontext`, **When** the user runs `rad deploy app.bicep`, **Then** the deployment targets `dev-rg`/`dev-env` and the config file is unchanged.
2. **Given** the same defaults, **When** the user runs `rad deploy app.bicep -g prod-rg -e prod-env`, **Then** the deployment targets `prod-rg`/`prod-env`, the config file is unchanged, and no prompt or warning about workspaces appears.
3. **Given** a config containing `workspaces.items.azure` with its own `connection.context`, `scope`, and `environment`, **When** the user runs `rad deploy app.bicep -w azure`, **Then** the deployment uses that workspace's connection, group, and environment as a unit; per-command `-g`/`-e` if also supplied still take precedence over the workspace's values.
4. **Given** no group default for the active context, no `--workspace`, no legacy `workspaces.default` resolving the key, **and** no resource group named `default` exists on the cluster, **When** the user runs `rad app list` without `--group`, **Then** the CLI fails with a remediation message that names `--group`, `--workspace`, and `rad configure --defaults`. (If a resource group named `default` does exist, the literal-`default` fallback satisfies the key and the command succeeds.)

---

### User Story 5 - Read-only back-compat with the legacy `workspaces:` block (Priority: P2)

An existing user upgrades the CLI without re-running `rad init`. Their `~/.rad/config.yaml` already contains a populated `workspaces:` block. Their workflows continue to work unchanged. The legacy block is read but never written by the new CLI; the management commands are gone but the data and the `--workspace` flag remain functional.

**Why this priority**: Keeps existing users from being broken on upgrade, while still moving the codebase to the new model.

**Independent Test**: Take an existing pre-refactor `~/.rad/config.yaml` (with `workspaces.default` and at least one item with `scope`, `environment`, `connection.context`). Upgrade the CLI. Run `rad app list` without modifying the file. Verify the command resolves group, environment, and Kubernetes connection from `workspaces.default`.

**Acceptance Scenarios**:

1. **Given** a pre-refactor config with `workspaces.default: default`, `workspaces.items.default.connection.context: my-kubecontext`, `workspaces.items.default.scope: /planes/radius/local/resourceGroups/foo`, and `workspaces.items.default.environment: /planes/radius/local/resourceGroups/foo/providers/Applications.Core/environments/bar`, **When** the user runs `rad app list`, **Then** the command targets group `foo`, environment `bar`, and the cluster reachable via kube context `my-kubecontext`, all resolved from `workspaces.default`.
2. **Given** the same config, **When** the user runs `rad configure --list-defaults`, **Then** the listing includes the values resolved from `workspaces.default`, clearly labels them as coming from the legacy block, and points the user at `rad configure --defaults group=...` if they want to migrate.
3. **Given** the same config, **When** the user runs `rad configure --defaults group=new-rg` while the active kube context is `my-kubecontext`, **Then** the CLI writes `defaults.my-kubecontext.group: new-rg`, **does not modify** the `workspaces:` block, and on subsequent commands `defaults.my-kubecontext.group` takes precedence over `workspaces.default.scope`.
4. **Given** no `defaults:` entry for the active kube context, no `workspaces.default`, and no resource group/environment named `default` on the cluster, **When** any scoped command runs without `-g`/`-e`/`-w`, **Then** it fails with the standard remediation message naming all three flags and `rad configure --defaults`. (If `default`/`default` resources exist, the literal fallback satisfies the keys and the command succeeds.)
5. **Given** the user invokes `rad workspace create/list/show/switch/delete`, `rad group switch`, or `rad env switch`, **When** they run, **Then** they fail with a "command removed" error naming the `rad configure --defaults` replacement and pointing at migration docs. The `--workspace <name>` flag, however, remains functional on scoped commands.

---

### User Story 6 - JSON / scripting friendliness (Priority: P3)

A user automating Radius operations in CI wants a stable, scriptable interface for reading and setting defaults.

**Why this priority**: Useful but not blocking; humans can use the table output during the rollout.

**Independent Test**: Run `rad configure --list-defaults --output json` and pipe to `jq`. Run `rad configure --defaults group=<name>` in CI without TTY and confirm non-interactive success when the target exists.

**Acceptance Scenarios**:

1. **Given** a configured CLI, **When** the user runs `rad configure --list-defaults --output json`, **Then** the output is valid JSON keyed by kube context with stable field names (`group`, `environment`, plus a `source` field of `"defaults"` or `"workspaces"`).
2. **Given** a CI environment with no TTY, **When** the user runs `rad configure --defaults group=my-rg`, **Then** the command succeeds without prompts when the group exists, and fails non-zero with a stable error code when it does not.

---

### Edge Cases

- **Cluster unreachable during `rad configure --defaults`**: Fail without mutating the config; clearly distinguish "cluster unreachable" from "group not found".
- **Group default set, environment unset, no `default` environment on cluster**: Commands needing only a group succeed; commands needing an environment fall through to the literal `default` fallback and fail with a precise "no default environment" message only if a `default` environment does not exist.
- **Both unset, fresh install before `rad init`**: `rad configure --list-defaults` prints an empty result and a hint to run `rad init`.
- **Stale environment value**: Default environment was deleted out of band. Next scoped command fails with a remediation (run `rad env list`, then `rad configure --defaults environment=<name>`).
- **Concurrent edits to `~/.rad/config.yaml`**: Two `rad configure --defaults …` invocations run in parallel; the file must not become corrupted (last-writer-wins with file locking is acceptable).
- **`rad init` re-run on an already-configured machine**: Existing defaults for the active context must not be silently overwritten without user confirmation.
- **Active kube context contains characters unusual in YAML keys** (dots, slashes, colons): These MUST be preserved verbatim as the YAML map key (quoted as needed).
- **Two contexts point at the same cluster**: Treated as independent entries in `defaults:`; the user can configure them identically or differently. No deduplication by Radius.
- **`KUBECONFIG` references multiple files / a non-default path**: The "active kube context" is resolved via the standard kubeconfig precedence rules used elsewhere in the CLI; no new resolution logic is introduced.
- **Legacy block contains a workspace whose `connection.context` matches the active kube context AND `defaults.<active-context>` exists**: `defaults:` always wins over `workspaces.default`, key by key. Missing keys fall through to `workspaces.default` per the resolution order — the two sources are merged per-key.
- **`-w <name>` names a workspace that does not exist in `workspaces.items`**: Command fails with a clear error before contacting the cluster.
- **`-w <name>` plus `-g`/`-e` flags**: `-g`/`-e` win for those keys; the workspace supplies the remaining keys (notably `connection.context`).
- **`-w` is set but the workspace has no `scope` or `environment`** (e.g., the `azure` entry in the user's example config): the workspace supplies only `connection.context`; group/environment must come from `-g`/`-e`/`defaults:`/`workspaces.default` per the standard resolution order, otherwise a clear error.

## Requirements *(mandatory)*

### Functional Requirements

#### Command surface

- **FR-001**: The CLI MUST expose a `rad configure` command. It MUST support `--defaults <key>=<value> [<key>=<value>…]` (set), `--defaults <key>=` with an empty value (clear that key), and `--list-defaults` (inspect). `--list-defaults` MUST support `--output json`.
- **FR-002**: Supported keys for `--defaults` MUST include `group` and `environment`. Unknown keys MUST fail the command with a message listing supported keys.
- **FR-003**: All `--defaults` operations MUST always target the entry for the **active Kubernetes context** in the new `defaults:` block. There is no flag to target a different context.
- **FR-004**: `rad configure --defaults group=<name>` MUST validate that the named resource group exists on the cluster reachable via the active kube context before persisting. `rad configure --defaults environment=<name>` MUST validate the environment exists within the configured (or already-set) default group.
- **FR-005**: When multiple keys are provided in one invocation (e.g., `group=… environment=…`), the operation MUST be atomic: validation of all keys MUST succeed before any write; on any failure, the file MUST be unchanged.
- **FR-006**: `rad configure --list-defaults` MUST display all entries from the `defaults:` block, plus any values resolved from the legacy `workspaces:` block (clearly labeled as such), with the active kube context highlighted.

#### Storage and schema

- **FR-007**: New commands MUST write defaults to a top-level `defaults:` block keyed by Kubernetes context name. Within each context entry, supported subkeys are `group` and `environment` (string values, names — not URIs).
- **FR-008**: New commands MUST NOT write to the legacy `workspaces:` block under any circumstance.
- **FR-009**: Setting a key to an empty value (clear) MUST remove that key from `defaults.<active-context>`. If the resulting context entry has no remaining keys, the entry itself MUST be removed. If the resulting `defaults:` block is empty, the block MUST be removed.
- **FR-010**: `rad configure --defaults …` operations MUST be safe under concurrent execution (no file corruption); last-writer-wins is acceptable. The file MUST be written atomically (write-temp-then-rename or equivalent).
- **FR-011**: On any validation failure, the config file MUST remain byte-for-byte unchanged.

#### Resolution and command behavior

- **FR-012**: All scoped commands (including but not limited to `rad deploy`, `rad app list/show/delete/connections/status/graph`, `rad env list/show/create/delete`, `rad group list/show/create/delete`, `rad resource list/show/delete`, `rad recipe list/show/register/unregister`, `rad credential …`) MUST resolve group, environment, and Kubernetes connection using this exact order, applied **per key independently**:
  1. Per-command `-g/--group` / `-e/--environment` flag.
  2. Per-command `-w/--workspace <name>` flag: if present, the named entry in `workspaces.items.<name>` supplies any keys (group from `scope`, environment from `environment`, connection from `connection.context`) not yet resolved by step 1.
  3. `defaults.<active-kube-context>.<key>` from the new `defaults:` block.
  4. `workspaces.default`: the workspace named by `workspaces.default` supplies any keys still unresolved (group from `scope`, environment from `environment`, connection from `connection.context`).
  5. **Built-in literal `default`**: for the `group` and `environment` keys only, the literal string `default` MUST be used if no earlier source supplied a value. This pairs with `rad init`'s creation of a resource group named `default` and an environment named `default` to deliver a working zero-config experience.
  6. Error — only reachable when the resolved Kubernetes connection cannot be determined, **or** when the user has explicitly cleared a default (e.g., via `rad configure --defaults group=`) and the literal `default` does not exist on the cluster. The remediation message MUST name `--group`, `--environment`, `--workspace`, and `rad configure --defaults`.
- **FR-013**: Per-key resolution MUST stop at the first source supplying a value for that key. Sources MAY supply different keys: e.g., `-g` from the command line, `environment` from `defaults:`, and `connection.context` from `workspaces.default` is a valid combined resolution.
- **FR-014**: When the Kubernetes connection cannot be determined from any source (no `-w`, no active kube context, no usable `workspaces.default`), any scoped command MUST fail fast with a remediation message and MUST NOT pick a default arbitrarily.
- **FR-015**: When the resolved Kubernetes connection refers to a cluster that is unreachable, command behavior MUST mirror today's behavior for unreachable clusters (this refactor does not change connection-failure semantics).

#### `rad init`

- **FR-016**: `rad init` MUST NOT create, name, or reference a "workspace" in any user-facing output.
- **FR-017**: `rad init` MUST create a resource group named `default` and an environment named `default` on the connected cluster (matching the literal-`default` fallback in FR-012 step 5). It MUST NOT write to `~/.rad/config.yaml` for the purpose of recording these defaults; the literal-`default` resolution rule provides the same effect with no file mutation. Users who want non-`default` names use `rad configure --defaults group=<name> environment=<name>` after `rad init`.
- **FR-017a**: `rad init` MAY still write `~/.rad/config.yaml` for non-default purposes (e.g., recording cloud-provider credentials or other configuration outside the `defaults:` and `workspaces:` blocks). Any such writes MUST NOT touch the `defaults:` or `workspaces:` blocks.

#### Removal of legacy commands

- **FR-018**: The following commands MUST be removed: `rad workspace create`, `rad workspace list`, `rad workspace show`, `rad workspace switch`, `rad workspace delete`, `rad group switch`, `rad env switch`. Invoking any of them MUST fail with a "command removed" error message that names the `rad configure --defaults` replacement and links to migration docs.
- **FR-019**: The `-w/--workspace <name>` **flag** on scoped commands MUST be **preserved**. It selects a named entry from `workspaces.items` for the duration of one command invocation per FR-012 step 2. Help text for the flag MUST describe it as a per-command override that reads the legacy `workspaces:` block.
- **FR-020**: The Go type `workspaces.Workspace` and related infrastructure MAY remain inside the codebase. It MUST NOT be exposed in any new public CLI surface other than the `--workspace` flag described in FR-019, and new help text/documentation MUST NOT introduce the term "workspace" outside the `--workspace` flag's own help and the migration guide.

#### Documentation and discoverability

- **FR-021**: All new help text, getting-started docs, and error messages MUST avoid introducing the term "workspace" to new users. Documentation MUST include a migration guide from `rad workspace`/`rad group switch`/`rad env switch` to `rad configure --defaults`, and from a populated `workspaces:` block to a `defaults:` block (showing equivalent commands).

### Key Entities

- **Defaults Entry**: A map keyed by Kubernetes context name. Each value contains the default `group` (resource group name) and `environment` (environment name) for `rad` commands run while that context is active. Stored under the new top-level `defaults:` key in `~/.rad/config.yaml`. Replaces the user-visible "workspace" concept.
- **Legacy Workspace Entry**: The pre-existing structure under `workspaces.items.<name>` containing `connection`, `scope`, and `environment`. Read-only after this refactor. Used (a) per-key as a fallback when `defaults.<active-context>` does not provide a value (the workspace named by `workspaces.default`), and (b) as a one-shot override when the user passes `-w <name>` on a command.
- **Active Kubernetes Context**: The current `current-context` resolved from the standard kubeconfig precedence chain. The lookup key for `defaults:`. The single source of truth for which cluster the CLI talks to.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After `rad init` on a clean machine, a user can run `rad app list` and `rad deploy app.bicep` to completion **without ever seeing or typing the word "workspace"**.
- **SC-002**: Help text and getting-started docs contain **zero occurrences** of "workspace" outside the migration guide.
- **SC-003**: A user with an unmodified pre-refactor `~/.rad/config.yaml` can upgrade the CLI and run their previous scoped workflows with **no manual file edits** and **no command failures** caused by schema changes (assuming the active kube context matches the legacy default workspace's `connection.context`).
- **SC-004**: A user with `defaults:` entries for two kube contexts can switch between contexts via `kubectl config use-context` and run scoped `rad` commands against each cluster **with zero additional `rad` configuration steps** between switches.
- **SC-005**: `rad configure --defaults …` either succeeds and persists the value(s), or fails and leaves the config file byte-for-byte unchanged, in **100%** of test runs (including network failures, validation failures, and unknown keys).
- **SC-006**: Every operation previously achievable via `rad workspace switch`, `rad group switch`, or `rad env switch` is achievable via a single `rad configure --defaults …` invocation; **no operation requires more steps after the refactor than before**.
- **SC-007**: Time-to-first-deploy for a new user (time from `rad init` start to a successful `rad deploy app.bicep`) does not increase versus the pre-refactor baseline.
- **SC-008**: Inside the codebase, references to the legacy workspace types are confined to one well-defined compatibility shim package; no scoped command's resolution path reads workspace fields directly.

## Assumptions

- **Schema is additive, not replacing**: A new top-level `defaults:` key is added. The existing `workspaces:` key is read for back-compat but never written. This honors the "leave schema as-is if possible" priority while delivering a clean new UX.
- **Kube-context-keyed defaults remove the mismatch problem**: Because the lookup key is the active kube context, switching contexts in `kubectl` automatically selects the matching defaults. There is no longer a "stored context vs. active context" mismatch to detect, prompt about, or auto-update. The earlier proposed FR for context-mismatch UX is therefore moot and dropped from this revision.
- **Hard removal of legacy management commands; flag preserved**: `rad workspace*`, `rad group switch`, and `rad env switch` are removed. The `-w/--workspace <name>` flag on scoped commands is **preserved** as a one-shot selector that reads the legacy `workspaces.items` block. This keeps existing scripts working and gives users a per-command escape hatch without bringing back the workspace-management surface.
- **One defaults entry per active kube context**: Multiple named "profiles" per context are out of scope. Users wanting multiple environments on the same cluster can use `--group`/`--environment` flags or maintain separate kube contexts.
- **Kubernetes-only connections in scope**: This refactor targets the Kubernetes connection kind. Non-Kubernetes connection kinds (none currently end-user-facing) are out of scope.
- **No new auth model**: Authentication against Kubernetes and Radius control planes is unchanged.
- **Tests as parity oracle**: Existing functional tests for scoped commands provide the parity oracle: every test that passes pre-refactor must pass post-refactor, with `--workspace` removed and `--group`/`--environment` flags or `defaults:` entries used in its place.
