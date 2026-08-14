# Modernizing the `rad init` Experience

- **Status**: In Review
- **Author**: Zach Casper (@zachcasper)

## Overview

`rad init` is the first command most people run when they try Radius. It installs
the Radius control plane onto a Kubernetes cluster, creates a default environment,
and writes local Bicep configuration so that the user can author and deploy an
application. Because it is the on-ramp to the product, its behavior disproportionately
shapes a new user's first impression.

Over the last several releases the command has accumulated behavior that no longer
matches how we want people to get started. It asks a confusing question, couples a
required configuration file (`bicepconfig.json`) to the answer, scaffolds a sample
`app.bicep` and a stub application resource that the getting-started flow no longer
needs, injects a Bicep extension (`aws`) that we do not want to encourage, overwrites
the current workspace entry in `~/.rad/config.yaml` when it points the CLI at a new
cluster, and renders at least one error so poorly that it corrupts the user's terminal.

Some of this erosion was incremental and well-intentioned. For example,
[#11766](https://github.com/radius-project/radius/pull/11766) removed the hidden
`.rad/rad.yaml` file that used to supply the current application name implicitly (the
right call for explicitness), but it left the scaffolded `app.bicep` unable to deploy
with a bare `rad deploy app.bicep`, because that file relies on an injected `application`
parameter with no application resource to satisfy it ([#12418](https://github.com/radius-project/radius/issues/12418)).
Individually reasonable changes like this have compounded into the rough on-ramp this
document sets out to fix.

This document enumerates the specific challenges (each backed by a filed issue),
then proposes a modernized two-mode experience: a zero-question `rad init --preview`
for the common case, and a fully interactive `rad init --preview --full` for platform
engineers who need to customize the environment, cloud providers, and recipe pack.

**Scope:** This proposal applies only to the preview (`Radius.*`) init code path under
[`pkg/cli/cmd/radinit/preview/`](../../../pkg/cli/cmd/radinit/preview/). The stable
`rad init` (`Applications.*`) path under [`pkg/cli/cmd/radinit/`](../../../pkg/cli/cmd/radinit/)
is out of scope and remains unchanged. Where the two paths share code (for example
[`pkg/cli/setup/application.go`](../../../pkg/cli/setup/application.go) and
[`common/`](../../../pkg/cli/cmd/radinit/common/)), changes are made additively so that
stable behavior is preserved.

## Terms and definitions

| Term                      | Definition                                                                                                                                                                                                                                                    |
|---------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Scaffold                  | The act of writing starter files (`app.bicep`, `bicepconfig.json`) into the current working directory during `rad init`.                                                                                                                                      |
| `bicepconfig.json`        | The Bicep configuration file that registers the Radius Bicep extension so the compiler can resolve Radius resource types. Required to author any Radius Bicep file.                                                                                           |
| Stub application resource | An `Applications.Core/applications` (stable) or `Radius.Core/applications` (preview) resource created by `rad init`, named after the current directory, with no resources of its own.                                                                         |
| Recipe pack               | A named, versioned collection of recipes (one per resource type) published by `resource-types-contrib` and referenced by an environment. The default pack Radius injects today is the `kubernetes` pack (its `Radius.Core/recipePacks` resource is named `default` in the contrib file), currently hardcoded in Go; `defaults.yaml` pins the `azure` and `kubernetes` packs. |
| `defaults.yaml`           | [`deploy/manifest/defaults.yaml`](../../../deploy/manifest/defaults.yaml) records the upstream `resource-types-contrib` revisions Radius pins for default resource types and recipe packs.                                                                  |
| Preview init              | The `rad init --preview` code path under [`pkg/cli/cmd/radinit/preview/`](../../../pkg/cli/cmd/radinit/preview/) that targets `Radius.*` resource types instead of `Applications.*`.                                                                          |
| `--full`                  | The flag that switches `rad init` from its opinionated defaults into a fully interactive prompt flow.                                                                                                                                                         |

## Objectives

> **Issue References:**
>
> - [#12588: No `bicepconfig.json` created after `rad init`](https://github.com/radius-project/radius/issues/12588)
> - [#12649: `rad init` creates an unnecessary stub application resource](https://github.com/radius-project/radius/issues/12649)
> - [#12587: Confusing user prompt on `rad init`](https://github.com/radius-project/radius/issues/12587)
> - [#12418: Sample `app.bicep` should define an application resource rather than an `application` parameter](https://github.com/radius-project/radius/issues/12418)
> - [#12568: Malformed error message when Kubernetes namespace does not exist during `rad init`](https://github.com/radius-project/radius/issues/12568)
> - [#12414: Remove AWS Bicep extension from `bicepconfig.json` scaffolded by `rad init`](https://github.com/radius-project/radius/issues/12414)
> - [#9177: Papercut: `rad init` overwrites `~/.rad/config.yaml`](https://github.com/radius-project/radius/issues/9177)

### Goals

- Make the default `rad init --preview` a zero-question command that always produces a
  working Radius installation and a valid `bicepconfig.json`.
- Stop producing artifacts the getting-started flow no longer needs: the sample
  `app.bicep` file and the stub application resource.
- Only register the `radius` Bicep extension in the generated `bicepconfig.json`.
- Never overwrite an existing workspace entry in `~/.rad/config.yaml` when `rad init --preview` targets a different cluster.
- Render initialization errors (especially the missing-namespace error) as concise,
  human-readable messages that do not corrupt the terminal.
- Keep `rad init --full` as the interactive path for platform engineers, and add a
  recipe-pack selection step sourced from `defaults.yaml` / `resource-types-contrib`.

> **Note:** Dropping the scaffolded `app.bicep` is only possible because the newly
> published getting-started guide now deploys the sample `app.bicep` directly from the
> [`samples`](https://github.com/radius-project/samples) repository by URL
> (`rad deploy https://raw.githubusercontent.com/radius-project/samples/refs/heads/v0.59/samples/demo/app.bicep`)
> rather than relying on a copy that `rad init --preview` writes into the working
> directory. The canonical sample therefore lives in one place and is versioned with the
> samples repo, so `rad init --preview` no longer needs to produce a local `app.bicep`.

### Non-goals

- Redesigning the environment, recipe-pack, or workspace **resource models**. This
  work changes CLI behavior only; it does not change server-side APIs beyond how the
  CLI calls them.
- Changing how the Radius control plane is installed via Helm.
- Authoring the recipe packs themselves in `resource-types-contrib`.

### User scenarios

#### User story 1: Developer trying Radius for the first time

As a developer evaluating Radius, I run `rad init --preview` and answer no questions. Radius is
installed and my workstation is completely ready to deploy an application with Radius. The getting-started guide then tells me to run
`rad deploy https://raw.githubusercontent.com/radius-project/samples/refs/heads/v0.59/samples/demo/app.bicep`,
and my sample app deploys.

#### User story 2: Platform engineer configuring a shared environment

As a platform engineer, I run `rad init --preview --full`. I am asked, in a logical order, which
Kubernetes context to use, whether to configure Azure/AWS cloud provider credentials,
the environment name, the Kubernetes namespace, and finally which recipe pack the new
environment should use. My existing workspaces in `~/.rad/config.yaml` are preserved.

## User Experience

### As-is experience (today)

Even the default `rad init --preview` is not truly zero-question: it always asks the
scaffold question, and `--full` interleaves that question among the others. Which prompts
fire in each mode today:

| Prompt                                      | Default `rad init --preview`   | `rad init --preview --full` |
|---------------------------------------------|--------------------------------|-----------------------------|
| Kubernetes context                          | skipped (uses current context) | asked                       |
| Environment name                            | skipped (`default`)            | asked                       |
| Kubernetes namespace                        | skipped (`default`)            | asked                       |
| Add a cloud provider?                       | skipped (none)                 | asked                       |
| Setup application in the current directory? | **asked**                      | **asked**                   |
| Application name (only if scaffolding)      | derived from directory name    | derived from directory name |

#### `rad init --preview` (default) today

**Sample Input:**

```bash
rad init --preview
```

**Sample Output:**

```text
? Setup application in the current directory? [Y/n] Yes

Initializing Radius. This may take a minute or two...

✅ Install Radius v0.59.0
   - Kubernetes cluster: kind-kind
   - Kubernetes namespace: radius-system
✅ Create new environment default
   - Kubernetes namespace: default
   - Recipe pack: default
✅ Scaffold application my-project
✅ Update local configuration

Initialization complete! Have a RAD time 😎
```

Answering **Yes** writes `app.bicep` and `bicepconfig.json` into the current directory
and creates a stub `applications` resource named after the current working directory (a potentially false assumption).

Answering **No** writes neither file, including the required `bicepconfig.json` (#12588), leaving the user's workstation with an incomplete installation.

#### `rad init --preview --full` today

**Sample Prompt Flow (current order):**

```text
Select the Kubernetes context to use             → kind-kind
Enter an environment name                        → default
Enter a namespace name to deploy apps into       → default
Add a cloud provider?                            → No / Azure / AWS
Setup application in the current directory?      → Yes
  (the application name is taken from the directory name, e.g. my-project; a name is
   only prompted for when the directory name is not a valid application name)
(a confirmation summary is shown before anything is applied)
```

Cloud-provider configuration comes **after** the environment prompts, and the scaffold
question sits at the end of the flow.

### To-be experience (proposed)

#### `rad init --preview` (default, zero questions)

**Sample Input:**

```bash
rad init --preview
```

**Sample Output:**

```text
Initializing Radius. This may take a minute or two...

✅ Install Radius v0.60.0
   - Kubernetes cluster: kind-kind
   - Kubernetes namespace: radius-system
✅ Create new environment default
   - Kubernetes namespace: default
   - Recipe pack: default
✅ Update local configuration
   - bicepconfig.json

Initialization complete! Have a RAD time 😎

Next step:
  Follow the Getting Started guide at https://docs.radapp.io/getting-started/deploy-demo/ for how to deploy the demo application.
```

Note the removed steps relative to today: there is **no** `Scaffold application <dir>`
line, **no** `app.bicep` written, and **no** stub application resource created. The
`Update local configuration` step now always writes `bicepconfig.json`.

#### `rad init --preview --full` (interactive)

**Sample Input:**

```bash
rad init --preview --full
```

**Sample Prompt Flow (new order):**

```text
Select the Kubernetes context to use                        → kind-kind
Add a cloud provider?                                       → Azure / AWS / [none]
  (cloud provider credential sub-flow, as-is)
Enter an environment name                                   → default
Enter a Kubernetes namespace name to deploy apps into       → default
Select a recipe pack for this environment                   → kubernetes / azure-aks / azure-aci
```

**Sample Output:**

```text
✅ Install Radius v0.60.0
   - Kubernetes cluster: kind-kind
   - Kubernetes namespace: radius-system
✅ Create new environment prod
   - Kubernetes namespace: prod
   - Recipe pack: azure-aks
✅ Update local configuration
   - bicepconfig.json

Initialization complete! Have a RAD time 😎
```

**Sample generated `bicepconfig.json` (both modes):**

```json
{
  "extensions": {
    "radius": "br:biceptypes.azurecr.io/radius:latest"
  }
}
```

## Design

### High Level Design

`rad init --preview` today runs a two-phase flow. `Validate()` gathers options
interactively via [`enterInitOptions()`](../../../pkg/cli/cmd/radinit/preview/options.go);
`Run()` then executes install → create environment → scaffold application → update
config. The four gather steps run in this order: cluster (kube context) → environment
(name, namespace) → cloud providers → application (the "Setup application in the current
directory?" prompt). See [`preview/options.go`](../../../pkg/cli/cmd/radinit/preview/options.go)
and [`preview/init.go`](../../../pkg/cli/cmd/radinit/preview/init.go).

The modernized design makes three structural changes:

1. **Decouple `bicepconfig.json` from scaffolding.** Writing `bicepconfig.json` moves
   out of the optional application-scaffold step and into an always-run configuration
   step, so it is created regardless of mode or answers.
2. **Delete the application-scaffold concept.** The `enterApplicationOptions` prompt,
   the `app.bicep` template, and the `CreateApplicationIfNotFound` stub-resource call
   are all removed. `ScaffoldApplication` is replaced by a narrower
   `WriteBicepConfig` that writes only `bicepconfig.json`.
3. **Reorder and extend `--full`.** Cloud-provider configuration moves before the
   environment prompts, and a new recipe-pack selection step is appended, sourced from
   `defaults.yaml`.

Two further changes are localized defect fixes: preserving existing workspaces (#9177)
and rendering initialization errors readably (#12568).

Because there are two parallel implementations (stable `Applications.*` and preview
`Radius.*`), all changes in this design are made in the preview implementation under
[`pkg/cli/cmd/radinit/preview/`](../../../pkg/cli/cmd/radinit/preview/), and shared code in
[`pkg/cli/cmd/radinit/common/`](../../../pkg/cli/cmd/radinit/common/) and
[`pkg/cli/setup/`](../../../pkg/cli/setup/) is changed only additively.

### The challenges (problem enumeration)

The challenges group into four themes. Each row states the issue, the observed
behavior, and the root cause in code. The same defects exist in the stable path, but per
the scope above this design fixes them in the preview (`Radius.*`) path only.

#### Theme A: The scaffold prompt is confusing and couples a required file to an optional answer

| Issue                                                           | Observed behavior                                                                                               | Root cause                                                                                                                                                             |
|-----------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [#12587](https://github.com/radius-project/radius/issues/12587) | The prompt "Setup application in the current directory?" is ambiguous; it does not say a file will be written. | [`common.EnterApplicationOptions`](../../../pkg/cli/cmd/radinit/common/application.go) presents an abstract "setup" question rather than naming the artifact.          |
| [#12588](https://github.com/radius-project/radius/issues/12588) | Answering **No** means no `bicepconfig.json` is created, leaving a non-working Bicep authoring setup.           | `bicepconfig.json` is written only inside [`ScaffoldApplication`](../../../pkg/cli/setup/application.go), which runs only when `options.Application.Scaffold` is true. |

The core defect is in [`ScaffoldApplication`](../../../pkg/cli/setup/application.go),
which writes **both** files atomically and is gated behind the scaffold answer:

```go
// pkg/cli/setup/application.go (today)
func ScaffoldApplication(directory string, template string) error {
    // writes app.bicep ...
    // writes bicepconfig.json ...
}
```

So the required file (`bicepconfig.json`) and the optional sample (`app.bicep`) share a
single gate. Answering "No" to a vaguely-worded prompt silently skips the required file.

#### Theme B: `rad init` produces artifacts the getting-started flow no longer needs

| Issue                                                           | Observed behavior                                                                                                                                                                                                                                                         | Root cause                                                                                                                                                     |
|-----------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [#12649](https://github.com/radius-project/radius/issues/12649) | `rad init` creates a stub `applications` resource named after the directory (the `Scaffold application test` step). It has no resources and serves no purpose.                                                                                                            | The preview [`Run()`](../../../pkg/cli/cmd/radinit/preview/init.go) creates a `Radius.Core/applications` stub during the scaffold step.                        |
| [#12418](https://github.com/radius-project/radius/issues/12418) | The scaffolded `app.bicep` declares an `application` parameter rather than an `applications` resource, so `rad deploy app.bicep` fails with "requires an application" after `.rad/rad.yaml` was removed in [#11766](https://github.com/radius-project/radius/pull/11766). | The `AppBicepTemplate` / `PreviewAppBicepTemplate` constants in [`application.go`](../../../pkg/cli/setup/application.go) use an injected `application` param. |
| [#12414](https://github.com/radius-project/radius/issues/12414) | The generated `bicepconfig.json` registers both `radius` and `aws` extensions, implying direct-to-AWS Bicep deployment is an encouraged pattern.                                                                                                                          | The `bicepConfigTemplate` constant in [`application.go`](../../../pkg/cli/setup/application.go) hardcodes both extensions.                                     |

Issue #12418 is worth calling out: it proposes *fixing* the `app.bicep` template to emit
a real `applications` resource. This design instead **removes** `app.bicep` entirely,
which supersedes #12418, since there is no scaffolded template left to fix. This is possible
because [#12676](https://github.com/radius-project/radius/pull/12676) lets `rad deploy`
accept a remote URL, so the getting-started guide can point directly at the sample:

```bash
rad deploy https://raw.githubusercontent.com/radius-project/samples/refs/heads/v0.59/samples/demo/app.bicep
```

#### Theme C: `rad init` overwrites the current workspace entry

| Issue                                                         | Observed behavior                                                                                                                                                                                               | Root cause                                                                                                                                                                                                                 |
|---------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [#9177](https://github.com/radius-project/radius/issues/9177) | With an existing current workspace, `rad init` replaces that workspace's `connection.context` and `environment` in `~/.rad/config.yaml` instead of creating a new one. Other workspace entries are left intact. | [`enterInitOptions`](../../../pkg/cli/cmd/radinit/preview/options.go) reuses the current workspace name (`ws.Name`) as the map key, so [`EditWorkspaces`](../../../pkg/cli/framework/config.go) replaces that one entry wholesale. |

The mechanism is a keyed replacement, not a whole-file overwrite:
[`cli.EditWorkspaces`](../../../pkg/cli/config.go) reads the workspace section and the
editor assigns `section.Items[name] = *workspace`, leaving every other entry intact. The
defect is the name-key reuse below: the key collides with the current workspace, so its
value is replaced instead of a new entry being added.

```go
// pkg/cli/cmd/radinit/preview/options.go (today)
if ws == nil {
    workspace.Name = "default"
} else {
    workspace.Name = ws.Name   // ← reuses the current workspace's name as the map key
}
```

The damage is cross-cluster. If the current workspace points at cluster A and the user
runs `rad init --preview` against cluster B, the workspace keeps its name but its
`connection.context` is repointed to B and its `environment` reset, silently destroying
its binding to A. This is the [#9177](https://github.com/radius-project/radius/issues/9177)
repro: a workspace on context `hollow` was repointed to `kind-init-bug` by an unrelated
`rad init`.

#### Theme D: Initialization errors are rendered unreadably

| Issue                                                           | Observed behavior                                                                                                                                                                                   | Root cause                                                                                                                                                                                                                                     |
|-----------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [#12568](https://github.com/radius-project/radius/issues/12568) | Supplying a non-existent namespace returns a valid `400`, but the raw JSON body is printed with the progress UI's indentation still active, corrupting terminal width until the terminal is killed. | The HTTP error body is surfaced verbatim while the bubbletea progress display owns the terminal; whitespace from the JSON is interleaved with the UI's hard-wrapping in [`common/display.go`](../../../pkg/cli/cmd/radinit/common/display.go). |

### Detailed Design

#### Change 1: Always write `bicepconfig.json`; never write `app.bicep`

Add a new, narrow helper alongside the existing `ScaffoldApplication` in the shared
[`pkg/cli/setup/application.go`](../../../pkg/cli/setup/application.go). `ScaffoldApplication`
and the stable `AppBicepTemplate` are left untouched so the stable path is unaffected:

```go
// pkg/cli/setup/application.go (proposed, additive)

// WriteBicepConfig writes a radius-only bicepconfig.json into directory if it does not
// already exist. It never overwrites an existing file (the user may have customized it).
func WriteBicepConfig(directory string) error {
    bicepConfigFilepath := filepath.Join(directory, "bicepconfig.json")
    if _, err := os.Stat(bicepConfigFilepath); err == nil {
        return nil // preserve user edits
    } else if !os.IsNotExist(err) {
        return err
    }
    return os.WriteFile(bicepConfigFilepath, []byte(getRadiusOnlyBicepConfig()), 0644)
}
```

- Emit **only** the `radius` extension (resolves #12414 for preview). The stable
  `radius` + `aws` template used by `ScaffoldApplication` is left in place:

  ```go
  const radiusOnlyBicepConfigTemplate = `{
      "extensions": {
          "radius": "br:biceptypes.azurecr.io/radius:%s"
      }
  }`
  ```

- Delete the preview-only `PreviewAppBicepTemplate` constant (resolves #12418 by
  removal). The stable `AppBicepTemplate` remains.
- In the preview [`Run()`](../../../pkg/cli/cmd/radinit/preview/init.go), replace the
  scaffold block (the `if r.Options.Application.Scaffold { ... }` section that creates the
  `Radius.Core/applications` stub and calls `ScaffoldApplication`) with an unconditional
  `WriteBicepConfig(wd)` in the configuration step. This resolves #12588 and #12649 for
  preview, and removes the stub-application creation.

#### Change 2: Remove the application-scaffold prompt and options (preview only)

- Delete `enterApplicationOptions` from
  [`pkg/cli/cmd/radinit/preview/application.go`](../../../pkg/cli/cmd/radinit/preview/application.go)
  and stop calling it from [`preview/options.go`](../../../pkg/cli/cmd/radinit/preview/options.go).
  The shared `EnterApplicationOptions` in
  [`common/application.go`](../../../pkg/cli/cmd/radinit/common/application.go) is **kept**
  because the stable path still uses it.
- Remove `applicationOptions` and the `Application` field from the preview `initOptions`
  in [`preview/options.go`](../../../pkg/cli/cmd/radinit/preview/options.go), and remove the
  preview `UpdateApplicationOptions`.
- In [`preview/display.go`](../../../pkg/cli/cmd/radinit/preview/display.go), stop
  populating `ScaffoldFiles` / `Application.Scaffold` in `toDisplayOptions`. The shared
  scaffold rendering in [`common/display.go`](../../../pkg/cli/cmd/radinit/common/display.go)
  stays in place (still used by the stable path) but is simply never triggered from preview.
- Delete the preview `application_test.go` and update
  [`preview/init_test.go`](../../../pkg/cli/cmd/radinit/preview/init_test.go) to assert that
  `app.bicep` is **not** written and that `bicepconfig.json` **is** written in every case,
  inverting the current `os.Stat(... "app.bicep")` assertions.

#### Change 3: Preserve existing workspaces (fix #9177, preview only)

In the preview [`enterInitOptions`](../../../pkg/cli/cmd/radinit/preview/options.go), only
reuse a workspace name when the selected Kubernetes context matches the existing
workspace's context. Otherwise choose a new, distinct workspace name (so the map key does
not collide) and set it as current. No change to `EditWorkspaces` is required: it already
reads the existing section and preserves other entries; the fix is solely in the name-key
selection, which today collides with and replaces the current entry.

#### Change 4: Reorder `--full` and add recipe-pack selection

Reorder the gather sequence in the preview `enterInitOptions`
([`preview/options.go`](../../../pkg/cli/cmd/radinit/preview/options.go)) so cloud-provider
configuration precedes the environment prompts:

```text
cluster (kube context)
  → cloud providers          (moved earlier)
  → environment name
  → environment namespace
  → recipe pack              (new)
```

Add a recipe-pack selection step. In default mode the environment is created with the
`kubernetes` pack (unchanged behavior); it is the default pack, and its `recipePacks`
resource is named `default` in the contrib file (hence the `Recipe pack: default` line in
the output). In `--full` mode, present a list built from the
`recipePacks` entries in [`defaults.yaml`](../../../deploy/manifest/defaults.yaml)
(today: `azure`, `kubernetes`, after [PR #12662](https://github.com/radius-project/radius/pull/12662): `azure-aci`, `azure-aks`, `kubernetes`).

The selected pack drives environment creation in
[`preview/environment.go`](../../../pkg/cli/cmd/radinit/preview/environment.go)
(`CreateEnvironment`). Today that function always links the hardcoded default pack:

```go
// today: always the hardcoded default pack
defaultPack := recipepack.NewDefaultRecipePackResource()
// ...
envProperties.RecipePacks = []*string{to.Ptr(recipepack.DefaultRecipePackID())}
```

The proposed flow resolves the chosen pack name to a `resource-types-contrib` source
via the `defaults.yaml` pin (`repo` + `ref`/`tag`), downloads/creates the corresponding
`Radius.Core/recipePacks` resource, and links it to the environment.

> **Dependency:** This flow assumes a recipe-pack source defines **only** a
> `Radius.Core/recipePacks` resource. Today the `resource-types-contrib` packs (for
> example `recipe-packs/azure/aks-recipepack.bicep`) also declare a
> `Radius.Core/environments` resource, so deploying a pack provisions an environment as a
> side effect and would collide with the environment the preview
> [`Run()`](../../../pkg/cli/cmd/radinit/preview/init.go) creates in `CreateEnvironment`.
> This change therefore depends on
> [resource-types-contrib#296](https://github.com/radius-project/resource-types-contrib/issues/296),
> which separates recipe-pack definitions from environment provisioning.

#### Change 5: Human-readable initialization errors (fix #12568)

When an init step fails, tear down the bubbletea progress UI **before** printing the
error, and surface a parsed, single-line message rather than the raw HTTP body. The
missing-namespace case should read:

```text
✗ Failed to create environment: namespace 'test' does not exist in the Kubernetes
  cluster. Create it (kubectl create namespace test) and re-run rad init.
```

This requires the error path in the preview [`Run()`](../../../pkg/cli/cmd/radinit/preview/init.go)
to (a) signal the progress goroutine to stop and restore stdout via the existing
`restoreStdout` deferral before writing, and (b) extract `error.message` from the
`clierrors` response instead of echoing the indented JSON body. Any change to the shared
progress display in `common/` must be additive so stable behavior is preserved.

### CLI Design

`rad init --preview` and `rad init --preview --full` keep their names and flags.
Behavioral changes (preview only):

| Aspect                        | Before                                        | After                               |
|-------------------------------|-----------------------------------------------|-------------------------------------|
| Application prompt            | "Setup application in the current directory?" | removed                             |
| `app.bicep`                   | written on "Yes"                              | never written                       |
| Stub application resource     | created                                       | never created                       |
| `bicepconfig.json`            | written on "Yes" only                         | always written                      |
| `bicepconfig.json` extensions | `radius` + `aws`                              | `radius` only                       |
| `--full` order                | cluster → env → cloud → app                   | cluster → cloud → env → recipe pack |
| Recipe pack (`--full`)        | not selectable                                | selectable from `defaults.yaml`     |
| Existing workspace            | may be overwritten                            | preserved                           |
| Init error rendering          | raw JSON, breaks terminal                     | concise, terminal-safe              |

### Implementation Details

- **Preview CLI (`pkg/cli/cmd/radinit/preview/`)** carries the bulk of the work: option
  struct changes, prompt removal, prompt reordering, recipe-pack prompt, and error rendering.
- **Shared CLI setup (`pkg/cli/setup/application.go`)**: add `WriteBicepConfig`
  (radius-only) and delete the preview-only `PreviewAppBicepTemplate`. `ScaffoldApplication`,
  `AppBicepTemplate`, and the `radius` + `aws` template used by the stable path are left
  intact.
- **Shared display (`pkg/cli/cmd/radinit/common/`)**: unchanged; preview simply stops
  populating the scaffold fields it renders.
- **Recipe packs (`pkg/cli/recipepack/`, `pkg/defaults`)**: extend selection to read
  `recipePacks` from `defaults.yaml` and resolve a chosen pack to its contrib source.
- **Preview config (`pkg/cli/cmd/radinit/preview/options.go`)**: fix workspace name-key
  selection so a preview init targeting a different cluster does not collide with and
  replace the current workspace entry. `EditWorkspaces` itself is unchanged.

No UCP, Deployment Engine, or Core RP server-side changes are required. The environment
and recipe-pack resources already exist; only the preview CLI's use of them changes.

## Test plan

- **Unit tests.** Update the `radinit/preview` suite: assert `app.bicep` is never
  created; assert `bicepconfig.json` is always created with only the `radius` extension;
  assert no stub application resource is created; assert the `--preview --full` prompt
  order; assert recipe-pack selection maps to the linked pack; assert an existing
  workspace is preserved. Add `setup` tests for the new `WriteBicepConfig` (create,
  don't-overwrite, radius-only extension).
- **Error rendering.** Add a test that a `400` from environment creation yields a
  single-line, non-indented message and restores stdout.
- **Functional tests.** Confirm no `rad init`-driven CI depends on a generated
  `app.bicep` (prior analysis found none in `radius-project`; the org-wide E2E flows in
  `resource-types-verification` supply their own `app.bicep`). Add a smoke test:
  `rad init --preview` then `rad deploy <remote sample URL>` succeeds end to end.
