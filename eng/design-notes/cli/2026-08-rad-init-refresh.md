# Refreshing the `rad init` Experience

- **Author**: Zach Casper (@zachcasper)

## Overview

`rad init` is the first command most people run when they try Radius. It installs the Radius control plane onto a Kubernetes cluster, creates a default environment, and writes local Bicep configuration so that the user can author and deploy an application. Because it is the on-ramp to the product, its behavior disproportionately shapes a new user's first impression.

Over time the command has accumulated several issues that, while individually minor, add up to a rough on-ramp. It asks a vague question ("Setup application in the current directory?"), can leave an installation without the critical `bicepconfig.json` configuration file, and creates a stub application resource that the getting-started flow no longer needs, amongst other smaller issues.

Some of this erosion was incremental and well-intentioned. For example, [#11766](https://github.com/radius-project/radius/pull/11766) removed the hidden `.rad/rad.yaml` file that used to supply the current application name implicitly (the right call for explicitness), but it left the scaffolded `app.bicep` unable to deploy with a bare `rad deploy app.bicep`, because that file relies on an injected `application` parameter with no application resource to satisfy it ([#12418](https://github.com/radius-project/radius/issues/12418)). Individually reasonable changes like this have compounded into the rough on-ramp this document sets out to fix.

This document enumerates the specific problems (most backed by a filed issue), then proposes a revised two-mode experience: a zero-question `rad init` for the common case, and a fully interactive `rad init --full` for platform engineers who need to customize the environment, cloud providers, and recipe pack.

**Scope:** This proposal applies only to the preview (`Radius.*`) init path, invoked today with `rad init --preview` (as shown in the command samples below); for brevity the prose refers to it simply as `rad init`. The stable `rad init` (`Applications.*`) path is out of scope and remains unchanged.

## Objectives

**Issue References:**

- [#12588: No `bicepconfig.json` created after `rad init`](https://github.com/radius-project/radius/issues/12588)
- [#12649: `rad init` creates an unnecessary stub application resource](https://github.com/radius-project/radius/issues/12649)
- [#12587: Confusing user prompt on `rad init`](https://github.com/radius-project/radius/issues/12587)
- [#12418: Sample `app.bicep` should define an application resource rather than an `application` parameter](https://github.com/radius-project/radius/issues/12418)
- [#12568: Malformed error message when Kubernetes namespace does not exist during `rad init`](https://github.com/radius-project/radius/issues/12568)
- [#12414: Remove AWS Bicep extension from `bicepconfig.json` scaffolded by `rad init`](https://github.com/radius-project/radius/issues/12414)

### Goals

- Make the default `rad init` a zero-question command that always produces a working Radius installation and a valid `bicepconfig.json`.
- Stop producing artifacts the getting-started flow no longer needs: the sample `app.bicep` file and the stub application resource.
- Only register the `radius` Bicep extension in the generated `bicepconfig.json`, dropping the `aws` extension.
- Render initialization errors (especially the missing-namespace error) as concise, human-readable messages that do not corrupt the terminal.
- Keep `rad init --full` as the interactive path for platform engineers, and add a recipe-pack selection step sourced from `defaults.yaml` / `resource-types-contrib`.
- Ensure no `radius` CI checks regress.

> **Note:** Dropping the scaffolded `app.bicep` is only possible because the newly published getting-started guide now deploys the sample `app.bicep` directly from the [`samples`](https://github.com/radius-project/samples) repository by URL (`rad deploy https://edge.docs.radapp.io/samples/demo/app.bicep`) rather than relying on a copy that `rad init` writes into the working directory. The canonical sample therefore lives in one place and is versioned with the samples repo, so `rad init` no longer needs to produce a local `app.bicep`.

### Non-goals

- Changing control plane components. This change is CLI only.
- Changing how the Radius control plane is installed via Helm.

### User scenarios

#### User story 1: Developer trying Radius for the first time

As a developer evaluating Radius, I run `rad init` and answer zero questions. Radius is installed and my workstation is completely ready to deploy an application with Radius. The getting-started guide then tells me:

```text
Next step:
  Follow the Getting Started guide at https://docs.radapp.io/getting-started/deploy-demo/ for how to deploy the demo application.
```

#### User story 2: Platform engineer configuring a shared environment

As a platform engineer, I run `rad init --full`. I am asked, in a logical order, which Kubernetes context to use, whether to configure Azure/AWS cloud provider credentials, the environment name, the Kubernetes namespace, and finally which recipe pack the new environment should use.

## As-is User Experience

The prompts depend on whether the `--full` flag is passed:

| Prompt                                      | Default `rad init`             | `rad init --full`           |
|---------------------------------------------|--------------------------------|-----------------------------|
| Kubernetes context                          | skipped (uses current context) | asked                       |
| Environment name                            | skipped (`default`)            | asked                       |
| Kubernetes namespace                        | skipped (`default`)            | asked                       |
| Add a cloud provider?                       | skipped (none)                 | asked                       |
| Setup application in the current directory? | **asked**                      | **asked**                   |
| Application name (only if scaffolding)      | derived from directory name    | derived from directory name |

### `rad init`

**Sample Input:**

```bash
rad init --preview
```

**Sample Output:**

```text
Setup application in the current directory? [Y/n] Yes

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

Answering **Yes** writes `app.bicep` and `bicepconfig.json` into the current directory and creates a stub `applications` resource named after the current working directory (a potentially false assumption).

Answering **No** writes neither file, including the required `bicepconfig.json` (#12588), leaving the user's workstation with an incomplete installation.

### `rad init --full` today

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

Cloud-provider configuration comes **after** the environment prompts, and the scaffold question sits at the end of the flow.

## To-be User Experience

### `rad init` (default, zero questions)

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
   - /home/user/my-project/bicepconfig.json

Initialization complete! Have a RAD time 😎

Next step:
  Follow the Getting Started guide at https://docs.radapp.io/getting-started/deploy-demo/ for how to deploy the demo application.
```

Note the removed steps relative to today: there is **no** `Scaffold application <dir>` line, **no** `app.bicep` written, and **no** stub application resource created. The `Update local configuration` step now always writes `bicepconfig.json`.

### `rad init --full` (interactive)

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
   - /home/user/my-project/bicepconfig.json

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

#### Recipe pack selection (`--full`)

Recipe pack selection is offered only in `rad init --full`. The default, zero-question flow always uses the `kubernetes` pack (whose `Radius.Core/recipePacks` resource is named `default`, which is why the output reads `Recipe pack: default`) and does not prompt.

At the `Select a recipe pack for this environment` prompt, `rad init` lists the recipe packs pinned in [`defaults.yaml`](../../../deploy/manifest/defaults.yaml) (for example `kubernetes`, `azure-aks`, and `azure-aci`) so the choices always match what the installed control plane can provision. The user selects exactly one.

The selection drives two steps:

1. **After Radius is installed,** Radius always creates the `default` recipe pack. If the selected pack is different, it is created **in addition**, as its own `Radius.Core/recipePacks` resource sourced from the `resource-types-contrib` revision pinned in `defaults.yaml`.
2. **When the environment is created** from the user's prompt responses, it is linked to **only** the selected recipe pack. The `default` pack stays installed for other environments but is not referenced by this one.

> **Dependency:** Recipe pack selection depends on [resource-types-contrib#296](https://github.com/radius-project/resource-types-contrib/issues/296), which separates recipe-pack definitions from environment provisioning. Until it is available, selecting a non-default pack would also provision a conflicting environment (see Change 5).

## Design

### High Level Design

`rad init` today runs a two-phase flow. `Validate()` gathers options interactively via [`enterInitOptions()`](../../../pkg/cli/cmd/radinit/preview/options.go); `Run()` then executes install → create environment → scaffold application → update config. The four gather steps run in this order: cluster (kube context) → environment (name, namespace) → cloud providers → application (the "Setup application in the current directory?" prompt). See [`preview/options.go`](../../../pkg/cli/cmd/radinit/preview/options.go) and [`preview/init.go`](../../../pkg/cli/cmd/radinit/preview/init.go).

The refreshed design makes six changes: decouple `bicepconfig.json` from scaffolding, register only the `radius` Bicep extension, delete the application-scaffold concept, reorder the `--full` flow, add a recipe-pack selection step, and render initialization errors readably. Each is specified under Detailed Design below, which states the issues it resolves.

`rad init` and `rad init --full` keep their names and flags; the following table summarizes the behavior changes:

| Aspect                        | Before                                        | After                               |
|-------------------------------|-----------------------------------------------|-------------------------------------|
| Application prompt            | "Setup application in the current directory?" | removed                             |
| `app.bicep`                   | written on "Yes"                              | never written                       |
| Stub application resource     | created                                       | never created                       |
| `bicepconfig.json`            | written on "Yes" only                         | always written                      |
| `bicepconfig.json` extensions | `radius` + `aws`                              | `radius` only                       |
| `--full` order                | cluster → env → cloud → app                   | cluster → cloud → env → recipe pack |
| Recipe pack (`--full`)        | not selectable                                | selectable from `defaults.yaml`     |
| Init error rendering          | raw JSON, breaks terminal                     | concise, terminal-safe              |

Because there are two parallel implementations (stable `Applications.*` and preview `Radius.*`), all changes in this design are made in the preview implementation under [`pkg/cli/cmd/radinit/preview/`](../../../pkg/cli/cmd/radinit/preview/), and shared code in [`pkg/cli/cmd/radinit/common/`](../../../pkg/cli/cmd/radinit/common/) and [`pkg/cli/setup/`](../../../pkg/cli/setup/) is changed only additively.

### Detailed Design

#### Change 1: Always write `bicepconfig.json`; never write `app.bicep`

Resolves #12588, #12649, and #12418.

Today `bicepconfig.json` is written only inside [`ScaffoldApplication`](../../../pkg/cli/setup/application.go), gated on the scaffold answer, so answering "No" leaves a non-working Bicep setup (#12588); the preview [`Run()`](../../../pkg/cli/cmd/radinit/preview/init.go) also creates a useless `Radius.Core/applications` stub named after the directory (#12649); and the scaffolded `app.bicep` declares an `application` parameter rather than an `applications` resource, so `rad deploy app.bicep` fails after `.rad/rad.yaml` was removed in [#11766](https://github.com/radius-project/radius/pull/11766) (#12418). This design removes `app.bicep` entirely, superseding the template fix proposed in #12418, which is safe because [#12676](https://github.com/radius-project/radius/pull/12676) lets `rad deploy` accept a remote sample URL.

Add a new, narrow helper alongside the existing `ScaffoldApplication` in the shared [`pkg/cli/setup/application.go`](../../../pkg/cli/setup/application.go). `ScaffoldApplication` and the stable `AppBicepTemplate` are left untouched so the stable path is unaffected:

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

- Delete the preview-only `PreviewAppBicepTemplate` constant. The stable `AppBicepTemplate` remains.
- In the preview [`Run()`](../../../pkg/cli/cmd/radinit/preview/init.go), replace the scaffold block (the `if r.Options.Application.Scaffold { ... }` section that creates the `Radius.Core/applications` stub and calls `ScaffoldApplication`) with an unconditional `WriteBicepConfig(wd)` in the configuration step. This removes the stub-application creation.

#### Change 2: Register only the `radius` Bicep extension

Resolves #12414: the generated `bicepconfig.json` hardcodes both the `radius` and `aws` extensions, implying direct-to-AWS Bicep deployment is an encouraged pattern. The `WriteBicepConfig` helper from Change 1 emits **only** the `radius` extension:

```go
const radiusOnlyBicepConfigTemplate = `{
    "extensions": {
        "radius": "br:biceptypes.azurecr.io/radius:%s"
    }
}`
```

`%s` is replaced at write time with the Radius version the CLI targets: `latest` for edge builds (as shown in the sample output above), or the pinned release version otherwise.

#### Change 3: Remove the application-scaffold prompt and options

Resolves #12587: the "Setup application in the current directory?" prompt is ambiguous and does not say a file will be written, because [`common.EnterApplicationOptions`](../../../pkg/cli/cmd/radinit/common/application.go) presents an abstract "setup" question rather than naming the artifact. With `bicepconfig.json` now written unconditionally (Change 1), the prompt and its options serve no purpose and are removed from preview.

- Delete `enterApplicationOptions` from [`pkg/cli/cmd/radinit/preview/application.go`](../../../pkg/cli/cmd/radinit/preview/application.go) and stop calling it from [`preview/options.go`](../../../pkg/cli/cmd/radinit/preview/options.go). The shared `EnterApplicationOptions` in [`common/application.go`](../../../pkg/cli/cmd/radinit/common/application.go) is **kept** because the stable path still uses it.
- Remove `applicationOptions` and the `Application` field from the preview `initOptions` in [`preview/options.go`](../../../pkg/cli/cmd/radinit/preview/options.go), and remove the preview `UpdateApplicationOptions`.
- In [`preview/display.go`](../../../pkg/cli/cmd/radinit/preview/display.go), stop populating `ScaffoldFiles` / `Application.Scaffold` in `toDisplayOptions`. The shared scaffold rendering in [`common/display.go`](../../../pkg/cli/cmd/radinit/common/display.go) stays in place (still used by the stable path) but is simply never triggered from preview.
- Delete the preview `application_test.go` and update [`preview/init_test.go`](../../../pkg/cli/cmd/radinit/preview/init_test.go) to assert that `app.bicep` is **not** written and that `bicepconfig.json` **is** written in every case, inverting the current `os.Stat(... "app.bicep")` assertions.

#### Change 4: Reorder `--full`

This change closes a usability gap in `--full`: cloud-provider setup currently comes after the environment prompts, so credentials are entered after the environment they apply to.

Reorder the gather sequence in the preview `enterInitOptions` ([`preview/options.go`](../../../pkg/cli/cmd/radinit/preview/options.go)) so cloud-provider configuration precedes the environment prompts:

```text
cluster (kube context)
  → cloud providers          (moved earlier)
  → environment name
  → environment namespace
  → recipe pack              (new)
```

#### Change 5: Add a recipe-pack selection step

This change closes a usability gap in `--full`: there is no way to choose a recipe pack, so every environment is created with the hardcoded default pack.

In default mode the environment is created with the `kubernetes` pack (unchanged behavior); it is the default pack, and its `recipePacks` resource is named `default` in the contrib file (hence the `Recipe pack: default` line in the output). In `--full` mode, present a list built from the `recipePacks` entries in [`defaults.yaml`](../../../deploy/manifest/defaults.yaml) (today: `azure`, `kubernetes`, after [PR #12662](https://github.com/radius-project/radius/pull/12662): `azure-aci`, `azure-aks`, `kubernetes`).

The selected pack drives environment creation in [`preview/environment.go`](../../../pkg/cli/cmd/radinit/preview/environment.go) (`CreateEnvironment`). Today that function always links the default pack:

```go
// today: always the hardcoded default pack
defaultPack := recipepack.NewDefaultRecipePackResource()
// ...
envProperties.RecipePacks = []*string{to.Ptr(recipepack.DefaultRecipePackID())}
```

The proposed flow resolves the chosen pack name to a `resource-types-contrib` source via the `defaults.yaml` pin (`repo` + `ref`/`tag`), downloads/creates the corresponding `Radius.Core/recipePacks` resource, and links it to the environment.

> **Dependency:** This flow assumes a recipe-pack source defines **only** a `Radius.Core/recipePacks` resource. Today the `resource-types-contrib` packs (for example `recipe-packs/azure/aks-recipepack.bicep`) also declare a `Radius.Core/environments` resource, so deploying a pack provisions an environment as a side effect and would collide with the environment the preview [`Run()`](../../../pkg/cli/cmd/radinit/preview/init.go) creates in `CreateEnvironment`. This change therefore depends on [resource-types-contrib#296](https://github.com/radius-project/resource-types-contrib/issues/296), which separates recipe-pack definitions from environment provisioning.

#### Change 6: Human-readable initialization errors (fix #12568)

Resolves #12568. Today a non-existent namespace returns a valid `400`, but the raw JSON body is printed while the bubbletea progress display still owns the terminal, corrupting terminal width until the terminal is killed.

When an init step fails, tear down the bubbletea progress UI **before** printing the error, and surface a parsed, concise message (no raw or indented JSON) rather than the raw HTTP body. The missing-namespace case should read:

```text
✗ Failed to create environment: namespace 'test' does not exist in the Kubernetes
  cluster. Create it (kubectl create namespace test) and re-run rad init.
```

This requires the error path in the preview [`Run()`](../../../pkg/cli/cmd/radinit/preview/init.go) to (a) signal the progress goroutine to stop and restore stdout via the existing `restoreStdout` deferral before writing, and (b) extract `error.message` from the `clierrors` response instead of echoing the indented JSON body. Any change to the shared progress display in `common/` must be additive so stable behavior is preserved.

### Implementation Details

The same changes, organized by the packages they touch:

- **Preview CLI (`pkg/cli/cmd/radinit/preview/`)** carries the bulk of the work: option struct changes, prompt removal, prompt reordering, recipe-pack prompt, and error rendering.
- **Shared CLI setup (`pkg/cli/setup/application.go`)**: add `WriteBicepConfig` (radius-only) and delete the preview-only `PreviewAppBicepTemplate`. `ScaffoldApplication`, `AppBicepTemplate`, and the `radius` + `aws` template used by the stable path are left intact.
- **Shared display (`pkg/cli/cmd/radinit/common/`)**: unchanged; preview simply stops populating the scaffold fields it renders.
- **Recipe packs (`pkg/cli/recipepack/`, `pkg/defaults`)**: extend selection to read `recipePacks` from `defaults.yaml` and resolve a chosen pack to its contrib source.

No UCP, Deployment Engine, or Core RP server-side changes are required. The environment and recipe-pack resources already exist; only the preview CLI's use of them changes.

## Test plan

- **Unit tests.** Update the `radinit/preview` suite: assert `app.bicep` is never created; assert `bicepconfig.json` is always created with only the `radius` extension; assert no stub application resource is created; assert the `--full` prompt order; assert recipe-pack selection maps to the linked pack. Add `setup` tests for the new `WriteBicepConfig` (create, don't-overwrite, radius-only extension).
- **Error rendering.** Add a test that a `400` from environment creation yields a concise, terminal-safe message (not the raw indented JSON body) and restores stdout.
- **Functional tests.** Add a **getting-started tutorial smoke test** that mirrors the documented [deploy-demo](https://docs.radapp.io/getting-started/deploy-demo/) flow end to end, so the on-ramp cannot regress now that the local `app.bicep` scaffold is gone. Run it as a dedicated CI job on a clean kind cluster (so `rad init --preview` performs the install itself, rather than reusing the suite that pre-installs via `rad install kubernetes`) and assert, in order: `rad init --preview` exits 0 and writes `bicepconfig.json`; `rad deploy` of the demo sample exits 0; `kubectl get deployment,service` shows the `demo-default` Deployment and `demo-default-web` Service `Ready`; and `rad application graph demo-default --preview` lists the `demo-default` container with its `apps/Deployment` and `core/Service`. Deploy from a pinned, versioned raw URL (`https://raw.githubusercontent.com/radius-project/samples/<release-tag>/samples/demo/app.bicep`) to keep the run deterministic, and add a separate check that the live `https://edge.docs.radapp.io/samples/demo/app.bicep` redirect resolves to that same file, so drift between the published tutorial and the pinned sample is caught.

### CI impact

These changes do not affect existing CI. The functional-test workflows ([`functional-test-noncloud.yaml`](../../../.github/workflows/functional-test-noncloud.yaml) and [`functional-test-cloud.yaml`](../../../.github/workflows/functional-test-cloud.yaml)) and the [`.github/scripts`](../../../.github/scripts/) helpers provision Radius with `rad install kubernetes`, not `rad init`, and the only `rad init`-related test ([`cli_test.go`](../../../test/functional-portable/cli/noncloud/cli_test.go)) drives the stable `radinit` runner, which is out of scope. No CI depends on a generated `app.bicep` (prior analysis found none in `radius-project`; the org-wide E2E flows in `resource-types-verification` supply their own `app.bicep`).
