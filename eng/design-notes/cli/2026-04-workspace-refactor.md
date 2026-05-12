# Topic: Replace Radius Workspaces with `rad configure --defaults`

* **Author**: Zach Casper (@zachcasper)
* **Feature Branch**: `workspace-refactor`
* **Status**: Draft
* **Tracking PR**: [radius-project/radius#11775](https://github.com/radius-project/radius/pull/11775)

## Topic Summary

Radius today exposes a CLI-only concept called a **workspace**, configured through `rad workspace create/list/switch/delete`. Despite the API-style verbs, a workspace is not a Radius API object — it is a client-side bundle stored in `~/.rad/config.yaml` that combines a Kubernetes connection, a default resource group, and a default environment. The naming and command shape causes recurring user confusion, the most common questions being "is a workspace an API resource?", "what is the difference between a workspace, a resource group, and an environment?", and "why does my workspace stop working when I switch clusters?".

This topic replaces the user-visible workspace concept with `rad configure --defaults group=<name> environment=<name>`, modeled on `az configure --defaults`. Defaults are scoped to the active Kubernetes context, so switching clusters with `kubectl config use-context` automatically selects the right defaults. The legacy `workspaces:` block in `~/.rad/config.yaml` is read for backward compatibility but never written by new commands, the `--workspace` flag is preserved as a one-shot selector, and the management commands (`rad workspace*`, `rad group switch`, `rad env switch`) are removed. New users never encounter the word "workspace"; existing scripts and config files keep working without manual migration.

### Top level goals

- Eliminate "workspace" as a user-visible concept in Radius CLI help, docs, and error messages for new users.
- Replace `rad workspace switch` / `rad group switch` / `rad env switch` with a single, discoverable `rad configure --defaults` surface.
- Make per-cluster defaults automatic by keying them on the active Kubernetes context, so `kubectl config use-context` is sufficient to switch Radius targets.
- Preserve backward compatibility for existing `~/.rad/config.yaml` files and for scripts that pass `--workspace <name>`.
- Deliver a working zero-config experience after `rad init` without writing a `defaults:` entry, by falling back to the literal name `default`.

### Non-goals (out of scope)

- Multiple named "profiles" per kube context. Users wanting multiple environments on the same cluster use `--group`/`--environment` flags or maintain separate kube contexts.
- Non-Kubernetes connection kinds. The refactor targets Kubernetes connections only.
- Changes to authentication against Kubernetes or the Radius control plane.
- Replacing or rewriting the existing `workspaces:` schema on disk; the legacy block is left in place and read-only.
- Server-side persistence of CLI defaults.

## User profile and challenges

### User persona(s)

The primary user is a **Radius CLI user** — application developers and platform engineers who interact with Radius through the `rad` command. The most affected sub-population is **new users** in the first hour of using Radius, because the workspace concept appears at `rad init` time and shapes their entire mental model. A secondary user is the **existing user** who already has a populated `~/.rad/config.yaml` and a set of scripts that depend on `--workspace`.

### Challenge(s) faced by the user

Workspaces are a fourth top-level concept on equal footing with apps, environments, and resource groups, but their only job is to remember two strings (a default group name and a default environment name) per cluster. This causes:

- **Mistaking workspaces for API objects.** `rad workspace create` reads like `rad app create`, so users look for workspaces in `rad resource list`, in the dashboard, or in cluster CRDs — where they do not exist.
- **Three concepts where two would do.** Docs introduce workspace, resource group, and environment, but only the latter two correspond to anything on the cluster.
- **Three switch commands instead of one.** Changing what the next `rad` command targets is spread across `rad workspace switch`, `rad group switch`, and `rad env switch`, with subtly different semantics and per-command `--workspace`/`--group`/`--environment` flags whose precedence is not obvious.
- **Silent breakage on cluster switch.** A workspace pins a specific kube context. If the user switches clusters with `kubectl config use-context`, the workspace either silently uses the old (now-wrong) context or fails opaquely.

The `az` CLI solves the same problem with a much smaller surface (`az configure --defaults group=my-rg location=westus2`), and Radius users already know that pattern from working with Azure.

### Positive user outcome

A new user runs `rad init`, then `rad deploy app.bicep`, and never types or sees the word "workspace". An experienced user changes the active kube context and their next `rad` command automatically targets the matching cluster's defaults, with no extra Radius step. An existing user upgrades the CLI and their pre-refactor config file and `--workspace`-using scripts keep working unchanged. The CLI surface for "what does my next command target?" collapses from three commands plus a confusing precedence chain into one command (`rad configure --defaults`) and a clear, per-key resolution order.

## Key scenarios

### Scenario 1: First-time user runs `rad init` without writing defaults

A new user runs `rad init`, which creates a resource group named `default` and an environment named `default` on the cluster. Subsequent `rad` commands resolve to those names via a built-in literal-`default` fallback, so the CLI does not need to write a `defaults:` block at all. No "workspace" is created, named, or mentioned.

### Scenario 2: Set, list, and clear defaults with `rad configure`

A user sets defaults with `rad configure --defaults group=<name> environment=<name>`, lists them with `rad configure --list-defaults`, and clears individual keys by setting them to an empty value. Validation runs against the live cluster and the operation is atomic (all-or-nothing).

### Scenario 3: Per-context defaults make kube context switches automatic

A user configures defaults for two kube contexts (`dev-cluster`, `prod-cluster`), switches between them with `kubectl config use-context`, and runs `rad` commands. Each invocation automatically picks up the right defaults for the active context — no warnings, no prompts, no `rad config set context` step.

### Scenario 4: Per-command flags override; `--workspace` remains a one-shot selector

Every scoped command supports `-g/--group`, `-e/--environment`, and `-w/--workspace` flags. Per-command `-g`/`-e` always win. `-w <name>` selects a named workspace from the legacy `workspaces.items` block as a single unit for the duration of one command. None of these flags mutate the config file.

### Scenario 5: Read-only backward compatibility with the legacy `workspaces:` block

An existing user upgrades the CLI without re-running `rad init`. Their populated `workspaces:` block keeps working. The block is read but never written by new commands; the management commands are gone but the data and the `--workspace` flag remain functional.

### Scenario 6: JSON-friendly defaults for scripting and CI

`rad configure --list-defaults --output json` returns a stable, scriptable schema. `rad configure --defaults …` runs non-interactively in CI, exiting non-zero with a stable error code on validation failure.

## Key dependencies and risks

- **Dependency: kubeconfig precedence.** "Active kube context" is resolved using the standard kubeconfig precedence chain already used elsewhere in the CLI. No new resolution logic is introduced; this refactor inherits whatever the rest of `rad` already does.
- **Dependency: cluster reachability for validation.** `rad configure --defaults group=<name>` validates against the live cluster before persisting. If the cluster is unreachable, the command fails without mutating the file. The error message must distinguish "cluster unreachable" from "group not found".
- **Risk: silently breaking existing scripts.** Removing `rad workspace`, `rad group switch`, and `rad env switch` will break scripts that call them. Mitigation: those commands fail with a "command removed" error that names the `rad configure --defaults` replacement and links to migration docs. The `--workspace <name>` flag is preserved on scoped commands so the most common scripted use is unaffected.
- **Risk: surprise from the literal-`default` fallback.** A user who explicitly clears a default expects "no default" behavior, but the literal-`default` fallback could quietly resolve to a `default` group that happens to exist. Mitigation: document the fallback prominently; have `rad configure --list-defaults` show when the literal fallback would apply; remediation messages name `--group`, `--workspace`, and `rad configure --defaults` so the user can correct course quickly.
- **Risk: codebase-wide reach of the legacy `Workspace` type.** `workspaces.Workspace` is referenced by many command paths today. A scattered, half-finished refactor risks two parallel resolution code paths. Mitigation: confine legacy workspace reads to a single compatibility shim package; assert via tests that no scoped command's resolution path reads workspace fields directly (SC-008).
- **Risk: concurrent edits to `~/.rad/config.yaml`.** Two `rad configure --defaults …` invocations in parallel must not corrupt the file. Mitigation: write atomically (temp-file + rename) with file locking; document last-writer-wins semantics.

## Key assumptions to test and questions to answer

- **Assumption:** keying defaults on the active Kubernetes context is more intuitive than naming a separate "profile" because users already think in terms of clusters/contexts. Validation: existing user feedback during the rollout; observe whether any user re-asks for a named-profile concept.
- **Assumption:** the literal-`default` fallback (FR-012 step 5) is preferable to writing a `defaults:` entry from `rad init`. The first-run experience stays clean and the config file stays empty until the user changes something. Validation: zero-config flow works in CI on a fresh machine; users do not file bugs about "where is my config file".
- **Assumption:** preserving only the `--workspace` flag (without the management commands) is enough backward compatibility for existing scripts. Validation: survey/issue-tracker review; functional tests covering the `-w <name>` path.
- **Assumption:** validating against the live cluster on `rad configure --defaults` is acceptable latency. Validation: measure round-trip time during implementation; if it is too slow for CI, add an opt-out flag.
- **Question (open):** how should `rad configure --list-defaults` label values that come from the legacy `workspaces:` block — as `source: workspaces` JSON field plus a visual hint in table output, or as a separate section? To be answered when the listing UI is implemented.
- **Question (open):** should `rad init` re-run on an already-configured machine prompt before touching anything related to defaults? Current intent is yes. To be confirmed during `rad init` work.

## Current state

`~/.rad/config.yaml` today contains a `workspaces:` block that combines a Kubernetes connection, a default resource group (as a fully-qualified URI), and a default environment (also a URI):

```yaml
workspaces:
  default: default
  items:
    default:
      connection: { context: my-kubecontext, kind: kubernetes }
      scope: /planes/radius/local/resourceGroups/default
      environment: /planes/radius/local/resourceGroups/default/providers/Applications.Core/environments/default
```

The user-facing surface is `rad workspace create/list/show/switch/delete` plus `rad group switch`, `rad env switch`, and per-command `--workspace`/`--group`/`--environment` flags whose precedence is documented but not always obvious in error messages. The `workspaces.Workspace` Go type is referenced widely across command implementations.

There is no prior in-flight investment in replacing this surface; this topic is the first concrete proposal.

## Details of user problem

> When I install Radius and run `rad init`, the CLI prints something about a "workspace". I assume a workspace is a Radius resource — like an app or an environment — and I look for it in `rad resource list` and in the dashboard. It's not there. I read the docs and discover that a workspace is something stored in `~/.rad/config.yaml` whose only job is to remember which resource group and environment to use by default. I now have to keep three concepts straight (workspace, resource group, environment) where only two of them are real things on the cluster.

> When I want to point my next `rad` command at a different group or environment, I have to choose between `rad workspace switch`, `rad group switch`, and `rad env switch`, plus per-command `--workspace`/`--group`/`--environment` flags. The precedence is not obvious from the error messages, and I am never sure whether the change I just made is for one command or persisted.

> When I switch clusters with `kubectl config use-context`, my Radius workspace silently keeps using the old context (because it pinned it at creation time). My next `rad` command either targets the wrong cluster or fails opaquely. I have to remember to run `rad workspace switch` separately, and I have to keep my workspaces and my kube contexts manually in sync.

> The result is that the workspace concept costs me time on day one, costs me confusion every time I switch clusters, and adds a fourth concept that I have to teach to anyone I onboard onto Radius.

## Desired user experience outcome

> After this is implemented, I can install Radius, run `rad init`, and immediately deploy an application without ever typing or seeing the word "workspace". When I want different defaults, I run one command — `rad configure --defaults group=<name> environment=<name>` — modeled on the `az configure --defaults` pattern I already know. When I switch Kubernetes clusters with `kubectl config use-context`, my Radius defaults follow automatically because they are keyed on the active context. When I need to override defaults for a single command, I pass `-g`, `-e`, or `-w` and the precedence is consistent and predictable. As a result, my mental model is two concepts (resource group, environment) instead of three, my cluster-switching is one command instead of two, and my onboarding doc is shorter.

### Detailed user experience

**Step 1 — `rad init` on a fresh machine.** The user installs the CLI, points `kubectl` at their cluster, and runs `rad init`. The CLI installs the Radius control plane and creates a resource group named `default` and an environment named `default` on the cluster. **No `~/.rad/config.yaml` is written at all** — the file is only created the first time the user runs `rad configure --defaults`. Success output contains no occurrence of "workspace".

**Step 2 — Deploy without configuration.** The user runs `rad deploy app.bicep`. The CLI resolves group and environment to the literal name `default` (FR-012 step 5), targeting the resources `rad init` just created. The deployment succeeds with no extra setup.

**Step 3 — Set non-default values when desired.** The user runs:

```bash
rad configure --defaults group=my-rg environment=my-env
```

Both keys are validated against the live cluster, persisted atomically under `defaults.<active-kube-context>` in `~/.rad/config.yaml`, and confirmed in the success output.

**Step 4 — Inspect defaults.** The user runs `rad configure --list-defaults`. The CLI prints all entries from the new `defaults:` block (grouped by kube context, with the active one highlighted) plus any values resolved from the legacy `workspaces:` block, clearly labeled. `--output json` returns the same data with stable field names.

**Step 5 — Clear a single key.** The user runs `rad configure --defaults environment=` (empty value). The `environment` key is removed from `defaults.<active-context>`. Subsequent commands fall through the resolution order. If the resulting context entry is empty, it is removed; if the resulting `defaults:` block is empty, the block is removed.

**Step 6 — Switch clusters with `kubectl`.** The user runs `kubectl config use-context prod-cluster && rad app list`. The command targets `prod-cluster` with whatever defaults are configured for that context, automatically — no Radius command in between, no warning, no prompt.

**Step 7 — One-off override.** The user runs `rad deploy app.bicep -g prod-rg -e prod-env`. The deployment targets `prod-rg`/`prod-env` for that single command; the config file is unchanged.

**Step 8 — Existing scripts keep working.** A pre-existing script that runs `rad deploy app.bicep -w azure` continues to work: the CLI resolves connection, group, and environment from the legacy `workspaces.items.azure` entry as a unit. The user gets a deprecation pointer to `rad configure --defaults` only when they invoke removed commands like `rad workspace switch`.

**Resolution order** (per key independently, applied by every scoped command):

1. Per-command `-g/--group` / `-e/--environment` flag.
2. Per-command `-w/--workspace <name>` flag (the named entry from `workspaces.items` supplies group/environment/connection for keys not yet resolved).
3. `defaults.<active-kube-context>.<key>` from the new `defaults:` block.
4. `workspaces.default` (the workspace named by `workspaces.default` supplies group/environment/connection for keys still unresolved).
5. Built-in literal `default` for the `group` and `environment` keys only.
6. Error — only when the active kube context is unset and no `-w` was passed, or when the user explicitly cleared a default and the literal `default` does not exist on the cluster. The remediation message names `--group`, `--environment`, `--workspace`, and `rad configure --defaults`.

## Breaking changes

This refactor removes user-visible CLI surface. The changes below are breaking; everything not listed here is preserved or unchanged.

| Command | Change | Replacement | Notes |
|---|---|---|---|
| `rad workspace create` | Removed | `rad configure --defaults group=<name> environment=<name>` | Validates against the live cluster before persisting. |
| `rad workspace list` | Removed | `rad configure --list-defaults` (also `--output json`) | Includes values from the legacy `workspaces:` block, clearly labeled. |
| `rad workspace show` | Removed | `rad configure --list-defaults` | No per-entry "show" verb. |
| `rad workspace switch` | Removed | `rad configure --defaults group=<name> environment=<name>`; for cluster switching, use `kubectl config use-context` | Defaults are keyed on the active kube context, so a `kubectl` switch is sufficient to switch Radius targets. |
| `rad workspace delete` | Removed | `rad configure --defaults group= environment=` (clear keys) | Clearing the last key removes the context entry; clearing the last entry removes the file. |
| `rad group switch` | Removed | `rad configure --defaults group=<name>` | Per-key default; persisted under the active kube context. |
| `rad env switch` | Removed | `rad configure --defaults environment=<name>` | Per-key default; persisted under the active kube context. |
| `~/.rad/config.yaml` `workspaces:` block (writes) | No longer written by any new command | New `defaults:` block | Existing `workspaces:` entries on disk are preserved and read as a fallback. |

**Preserved (not breaking):**

- The `-w/--workspace <name>` flag on every scoped command. It continues to read `workspaces.items.<name>` from the legacy block.
- The `~/.rad/config.yaml` `workspaces:` block on disk. Existing entries keep working as a fallback in the resolution chain.
- The `-g/--group` and `-e/--environment` flags on every scoped command.

**User impact:**

- Scripts that call any removed subcommand fail with a "command removed" error that names the `rad configure --defaults` replacement and links to migration docs. They will not silently misbehave.
- Scripts that pass `-w <name>` keep working unchanged.
- Existing config files keep working unchanged. No migration step is required to upgrade.

## Key investments

### Feature 1 — `rad configure --defaults` command surface

A new `rad configure` command supporting `--defaults <key>=<value> [<key>=<value>…]` (set), `--defaults <key>=` with empty value (clear), and `--list-defaults` (inspect, with `--output json`). Supported keys are `group` and `environment`; unknown keys fail with a message listing the supported set. All `--defaults` operations target the active kube context only — there is no flag to target a different context. Multi-key invocations are atomic: validation succeeds for every key before any write, and any failure leaves the file unchanged.

### Feature 2 — `defaults:` storage block

A new top-level `defaults:` block in `~/.rad/config.yaml`, keyed by Kubernetes context name, with `group` and `environment` subkeys (string names, not URIs). Writes are atomic (temp-file + rename) and safe under concurrent execution (last-writer-wins acceptable). Clearing the last key in a context entry removes the entry; clearing the last entry removes the block.

### Feature 3 — Per-key resolution chain shared by all scoped commands

All scoped commands (`rad deploy`, `rad app *`, `rad env *`, `rad group *`, `rad resource *`, `rad recipe *`, `rad credential *`, …) resolve group, environment, and Kubernetes connection through the precedence order in _Detailed user experience → Resolution order_, applied per key independently. A single shared resolver implements this so no scoped command reads workspace fields directly (SC-008).

### Feature 4 — `rad init` zero-config flow

`rad init` creates a resource group named `default` and an environment named `default` on the connected cluster. It does not create or write `~/.rad/config.yaml` at all — the literal-`default` resolution rule (Feature 3, step 5) provides a working zero-config experience with no file on disk. The config file is created the first time the user runs `rad configure --defaults`. No user-facing output mentions "workspace".

### Feature 5 — Removal of legacy management commands; preservation of `--workspace`

`rad workspace create/list/show/switch/delete`, `rad group switch`, and `rad env switch` are removed. Invocations fail with a "command removed" error that names the `rad configure --defaults` replacement and links to migration docs. The `-w/--workspace <name>` flag on scoped commands is **preserved** as a per-command, one-shot selector that reads the legacy `workspaces.items` block. The Go `workspaces.Workspace` type may remain inside the codebase, confined to a compatibility shim package, but is not exposed in any new CLI surface other than the `--workspace` flag.

### Feature 6 — Read-only backward compatibility with the legacy `workspaces:` block

The existing `workspaces:` block is read but never written by new commands. The workspace named by `workspaces.default` supplies any keys still unresolved after `defaults:` (Feature 3, step 4). `rad configure --list-defaults` surfaces values from this source with a clear label and a hint to migrate. New commands writing to the file leave the `workspaces:` block byte-for-byte unchanged.

### Feature 7 — Documentation, error messages, and migration guide

All new help text, getting-started docs, and error messages avoid introducing the term "workspace" to new users. Documentation includes a migration guide from `rad workspace`/`rad group switch`/`rad env switch` to `rad configure --defaults`, and from a populated `workspaces:` block to a `defaults:` block, with side-by-side equivalents. Remediation messages on resolution failure name `--group`, `--environment`, `--workspace`, and `rad configure --defaults` together so the user can pick the right escape hatch.

## Detailed Requirements (appendix)

The detailed FRs and acceptance scenarios that the implementation tracks against are kept here for reference; they expand on _Key investments_ and _Detailed user experience_ above.

### Functional requirements

#### Command surface

- **FR-001** The CLI MUST expose a `rad configure` command supporting `--defaults <key>=<value> [<key>=<value>…]` (set), `--defaults <key>=` (clear), and `--list-defaults` (inspect). `--list-defaults` MUST support `--output json`.
- **FR-002** Supported keys for `--defaults` MUST include `group` and `environment`. Unknown keys MUST fail with a message listing supported keys.
- **FR-003** All `--defaults` operations MUST target the entry for the active Kubernetes context. There is no flag to target a different context.
- **FR-004** `rad configure --defaults group=<name>` MUST validate that the named resource group exists on the cluster reachable via the active kube context before persisting. `rad configure --defaults environment=<name>` MUST validate the environment exists within the configured (or already-set) default group.
- **FR-005** When multiple keys are provided in one invocation, the operation MUST be atomic: validation of all keys MUST succeed before any write; on any failure, the file MUST be unchanged.
- **FR-006** `rad configure --list-defaults` MUST display all entries from the `defaults:` block, plus any values resolved from the legacy `workspaces:` block (clearly labeled), with the active kube context highlighted.

#### Storage and schema

- **FR-007** New commands MUST write defaults to a top-level `defaults:` block keyed by Kubernetes context name. Within each context entry, supported subkeys are `group` and `environment` (string names, not URIs).
- **FR-008** New commands MUST NOT write to the legacy `workspaces:` block.
- **FR-009** Setting a key to an empty value MUST remove that key from `defaults.<active-context>`. If the resulting context entry has no remaining keys, the entry itself MUST be removed. If the resulting `defaults:` block is empty, the block MUST be removed.
- **FR-010** `rad configure --defaults …` operations MUST be safe under concurrent execution (no file corruption); last-writer-wins is acceptable. The file MUST be written atomically.
- **FR-011** On any validation failure, the config file MUST remain byte-for-byte unchanged.

#### Resolution and command behavior

- **FR-012** All scoped commands MUST resolve group, environment, and Kubernetes connection using the per-key precedence order described in _Detailed user experience → Resolution order_.
- **FR-013** Per-key resolution MUST stop at the first source supplying a value for that key. Sources MAY supply different keys.
- **FR-014** When the Kubernetes connection cannot be determined from any source, any scoped command MUST fail fast with a remediation message and MUST NOT pick a default arbitrarily.
- **FR-015** When the resolved Kubernetes connection refers to a cluster that is unreachable, command behavior MUST mirror today's behavior for unreachable clusters (this refactor does not change connection-failure semantics).

#### `rad init`

- **FR-016** `rad init` MUST NOT create, name, or reference a "workspace" in any user-facing output.
- **FR-017** `rad init` MUST create a resource group named `default` and an environment named `default` on the connected cluster (matching the literal-`default` fallback). It MUST NOT create or write `~/.rad/config.yaml`. The config file is created only on the first invocation of `rad configure --defaults`.

#### Removal of legacy commands

- **FR-018** `rad workspace create/list/show/switch/delete`, `rad group switch`, and `rad env switch` MUST be removed. Invoking any of them MUST fail with a "command removed" error naming the `rad configure --defaults` replacement and linking to migration docs.
- **FR-019** The `-w/--workspace <name>` flag on scoped commands MUST be preserved. It selects a named entry from `workspaces.items` for the duration of one command invocation per FR-012 step 2. Help text MUST describe it as a per-command override that reads the legacy `workspaces:` block.
- **FR-020** The Go type `workspaces.Workspace` and related infrastructure MAY remain inside the codebase, confined to a compatibility shim package. New help text and documentation MUST NOT introduce the term "workspace" outside the `--workspace` flag's own help and the migration guide.

#### Documentation and discoverability

- **FR-021** All new help text, getting-started docs, and error messages MUST avoid introducing the term "workspace" to new users. Documentation MUST include a migration guide.

### Acceptance scenarios

The acceptance scenarios that map one-to-one to the user stories in _Key scenarios_ are tracked alongside the implementation tasks. The most important of them:

1. After `rad init` on a clean machine, `rad app list` and `rad deploy app.bicep` succeed via the literal-`default` fallback with no `defaults:` written.
2. `rad configure --defaults group=my-rg environment=my-env` validates and persists atomically; a single bad key leaves the file unchanged.
3. With `defaults.dev-cluster` and `defaults.prod-cluster` configured, `kubectl config use-context prod-cluster && rad app list` automatically targets `prod-cluster` with no Radius step in between.
4. `rad deploy app.bicep -g prod-rg -e prod-env` overrides defaults for that command without mutating the config file.
5. A pre-refactor `~/.rad/config.yaml` keeps working unchanged; `rad workspace switch` fails with a "command removed" error naming `rad configure --defaults`; `rad deploy -w azure` continues to work.
6. `rad configure --list-defaults --output json` returns valid JSON keyed by kube context with stable field names (`group`, `environment`, `source`).

### Edge cases

- Cluster unreachable during `rad configure --defaults`: fail without mutating the config; distinguish "cluster unreachable" from "group not found".
- Group default set, environment unset, no `default` environment on cluster: commands needing only a group succeed; commands needing an environment fall through to literal `default` and fail with a precise message only if no `default` environment exists.
- Both unset, fresh install before `rad init`: `rad configure --list-defaults` prints empty and hints to run `rad init`.
- Stale environment value: default environment was deleted out of band. Next scoped command fails with a remediation (run `rad env list`, then `rad configure --defaults environment=<name>`).
- Concurrent edits: file must not corrupt; last-writer-wins with file locking is acceptable.
- `rad init` re-run on an already-configured machine: existing defaults for the active context must not be silently overwritten without confirmation.
- Active kube context contains characters unusual in YAML keys: preserved verbatim, quoted as needed.
- Two contexts pointing at the same cluster: independent entries; no deduplication.
- `KUBECONFIG` references multiple files: standard kubeconfig precedence; no new resolution logic.
- Legacy block contains a workspace whose `connection.context` matches the active kube context AND `defaults.<active-context>` exists: `defaults:` always wins per key; missing keys fall through to `workspaces.default` per the resolution order.
- `-w <name>` names a workspace not in `workspaces.items`: command fails with a clear error before contacting the cluster.
- `-w <name>` plus `-g`/`-e`: `-g`/`-e` win for those keys; the workspace supplies the remaining keys (notably `connection.context`).
- `-w` set but the workspace has no `scope` or `environment`: the workspace supplies only `connection.context`; group/environment must come from `-g`/`-e`/`defaults:`/`workspaces.default`, otherwise a clear error.

### Success criteria

- **SC-001** After `rad init` on a clean machine, a user can run `rad app list` and `rad deploy app.bicep` to completion without ever seeing or typing the word "workspace".
- **SC-002** Help text and getting-started docs contain zero occurrences of "workspace" outside the migration guide.
- **SC-003** A user with an unmodified pre-refactor `~/.rad/config.yaml` can upgrade the CLI and run their previous scoped workflows with no manual file edits and no command failures caused by schema changes.
- **SC-004** A user with `defaults:` entries for two kube contexts can switch between contexts via `kubectl config use-context` and run scoped `rad` commands against each cluster with zero additional `rad` configuration steps between switches.
- **SC-005** `rad configure --defaults …` either succeeds and persists the value(s), or fails and leaves the config file byte-for-byte unchanged, in 100% of test runs.
- **SC-006** Every operation previously achievable via `rad workspace switch`, `rad group switch`, or `rad env switch` is achievable via a single `rad configure --defaults …` invocation; no operation requires more steps after the refactor than before.
- **SC-007** Time-to-first-deploy for a new user does not increase versus the pre-refactor baseline.
- **SC-008** Inside the codebase, references to the legacy workspace types are confined to one well-defined compatibility shim package; no scoped command's resolution path reads workspace fields directly.
