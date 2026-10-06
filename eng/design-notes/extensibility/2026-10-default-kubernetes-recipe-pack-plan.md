# Default Kubernetes recipe pack: one definition, pinned (#11959 items 1 and 2)

Related: #11959, #11835, radius-project/resource-types-contrib#290, radius-project/resource-types-contrib#357. Follow-up, not in this plan: #13210 (default pack read-only, created by the control plane).

---

## 1. Background: how it works today

### 1.1 Two repos

- `radius-project/resource-types-contrib` ("contrib") holds resource type manifests (`<Namespace>/<type>/<type>.yaml`), recipes (Bicep/Terraform), and recipe packs (`recipe-packs/<pack>/`).
- `radius-project/radius` ("radius") consumes them.

### 1.2 Recipe images and their tags

contrib's `publish-bicep-recipes.yaml` builds each Kubernetes recipe and pushes it to `ghcr.io/radius-project/kube-recipes/<name>` with these tags:

| Tag            | When it is set                                              | Moves?                        |
|----------------|-------------------------------------------------------------|-------------------------------|
| `<commit SHA>` | Every publish                                               | No                            |
| `edge`         | Every push to contrib `main`                                | Yes                           |
| `<version>`    | Manual run with `release_version` (Radius release Step 7.3) | No                            |
| `latest`       | Same manual run as `<version>`                              | Yes, only at a Radius release |

So `:latest` always means "recipes as of the last Radius release", never edge.

### 1.3 Pins in `deploy/manifest/defaults.yaml`

A pin is one entry that points at a commit in contrib:

```yaml
recipePacks:
  - name: kubernetes
    repo: github.com/radius-project/resource-types-contrib
    ref: 18142182e52e19a46b0ed172037357e8e142dcd2   # commit that is fetched
    tag: "recipe-pack/kubernetes/v0.2.0"            # release it came from; "" if none
```

Two sections use this shape: `resourceTypes` (one entry per namespace, e.g. `Radius.Compute`) and `recipePacks` (one entry per pack: `azure-aks`, `azure-aci`, `kubernetes`).

### 1.4 How a pin is updated

Terms:

- "Unit": one thing that has a pin. Either a namespace (e.g. `Radius.Compute`) or a recipe pack (e.g. `kubernetes`).
- "Stable release": a git tag in contrib such as `recipe-pack/kubernetes/v0.3.0` or `Radius.Compute/v0.3.0`. Tags with a suffix such as `-rc.1` are prereleases and are ignored.

Example: Radius team releases the Kubernetes pack as v0.3.0 (task T2).

1. contrib: the "Release Recipe Pack" workflow creates tag `recipe-pack/kubernetes/v0.3.0` and a GitHub Release.
2. contrib: the release triggers `notify-radius.yaml`. It calls the GitHub API to start a workflow in radius and passes `{name: kubernetes, ref: recipe-pack/kubernetes/v0.3.0}`.
3. radius: `.github/workflows/update-resource-types.yaml` starts. It runs `make update-resource-types-and-recipe-packs`, which edits `defaults.yaml`:

   ```yaml
   - name: kubernetes
     ref: <commit SHA of that tag>
     tag: "recipe-pack/kubernetes/v0.3.0"
   ```

   It commits that to branch `bot/update-resource-types` and opens a PR titled `chore(resource-types-contrib): updates` (or updates the open one).
4. radius: a maintainer reviews and merges the PR. The pin is now on `main`.

Which commit the script writes (`build/scripts/sync-resource-types.sh`):

- The unit has at least one stable release: the commit of its newest stable tag. `tag` = that tag name.
- The unit has no stable release yet: the commit that was pushed to contrib `main` (step 1 is then a push to `main`, not a release). `tag` = `""`. Today `azure-aks` and `azure-aci` are like this.
- Once a unit has a stable release, pushes to contrib `main` no longer change its pin. Only new releases do.

### 1.5 What is copied into radius

- Resource types: pinned AND copied. `make sync-resource-types` copies each manifest into `deploy/manifest/built-in-providers/{dev,self-hosted}/`. The control plane registers them at startup. CI (`verify-resource-types-manifest.yaml`) re-runs the copy and fails on any diff ("drift check").
- Recipe packs: pinned only. The sync checks that `recipe-packs/<pack>/` exists at the pinned commit (`copy_manifests`, "Verified recipe pack") and copies nothing. Nothing in radius `main` reads the `recipePacks` pins today. The deploy workflows that used to read them (`.github/extension/`) moved to radius-project/ai-extensions in #12719 (Aug 31), and ai-extensions does its own pack handling.
- Consequence: after T3 the new pin is recorded but unused. `rad` starts using it only after PR-B.

### 1.6 How `rad` creates the default pack today

- Code: `pkg/cli/recipepack/recipepack.go`.
  - `GetCoreTypesRecipeInfo()` is a hard-coded Go list of 8 resource types.
  - `resolveRecipeTag(type, isEdge)` picks the tag: edge build -> `edge`; release build -> the commit SHA of the type's namespace pin in `resourceTypes`; namespace missing -> `edge`.
  - `NewDefaultRecipePackResource()` turns the list into a `RecipePackResource`.
  - `GetOrCreateDefaultRecipePack()` does GET, and PUT on 404.
- Callers: `rad init --preview` (`pkg/cli/cmd/radinit/preview/environment.go`), `rad env create --preview` (`pkg/cli/cmd/env/create/preview/create.go`), `rad deploy` (`pkg/cli/cmd/deploy/deploy.go`).
- Pack ID: `/planes/radius/local/resourceGroups/default/providers/Radius.Core/recipePacks/default`.

### 1.7 The problem

|                                     | `rad` (Go list)      | contrib `recipe-packs/kubernetes/default.bicep` |
|-------------------------------------|----------------------|-------------------------------------------------|
| Resource types                      | 8                    | 7 (no PostgreSQL)                               |
| Tag on release build                | namespace commit SHA | `:latest`                                       |
| Tag on edge build                   | `:edge`              | `:latest`                                       |
| Reads `recipePacks.kubernetes` pin? | No                   | n/a                                             |

- Item 2: the two lists differ. Registering the contrib pack by hand drops PostgreSQL.
- Item 1: `pkg/defaults` parses `recipePacks` but never uses it. Moving the `kubernetes` pin does not change what `rad` creates.

---

## 2. Goal

- `rad` builds the default Kubernetes pack from the pinned contrib pack file.
- The contrib pack and the pack `rad` creates contain the same resource types, recipe images, and parameters.
- The Go list in `recipepack.go` is deleted.

## 3. Decisions (agreed)

- D1 Tags. The pack file supplies resource types, recipe image names, and parameters. `rad` sets the tag exactly as today:
  - Radius release build: namespace commit pin from `resourceTypes`.
  - Radius edge build: `:edge`.
  - The contrib pack file keeps `:latest` for users who register it by hand (latest released recipes, see 1.2).
- D2 Copy. The sync copies the pinned Kubernetes pack into radius, compiled to JSON, under `deploy/manifest/recipe-packs/kubernetes/`. The Azure packs stay pin-only. The "not vendored" comments only existed because nothing needed a copy.
- D3 File name. The pack is `default-recipepack.bicep` at v0.2.0 and `default.bicep` on contrib `main`. The copy step lands after the pin moves to v0.3.0, so the sync reads only `default.bicep` and fails if it is missing.

## 4. Out of scope

- Who owns the default pack; `rad init` overwriting it; GET-then-PUT race (#13210).
- Digest references in recipe sources (#13166).
- `Radius.Compute/containerImages` in the pack (contrib#359, #13176).
- Azure packs.

---

## 5. Tasks

Order: T1 -> T2 -> T3 -> PR-A -> T5 -> PR-B -> T6, T7 -> T8. T4 any time before PR-B.

| ID   | Repo    | Summary                                             |
|------|---------|-----------------------------------------------------|
| T1   | contrib | Add PostgreSQL to the Kubernetes pack               |
| T2   | contrib | Release `recipe-pack/kubernetes/v0.3.0`             |
| T3   | radius  | Review and merge the pin bump bot PR                |
| T4   | radius  | Golden test of today's default pack                 |
| PR-A | radius  | Sync copies and compiles the pinned pack            |
| T5   | radius  | Sync test cases for the pack copy                   |
| PR-B | radius  | `rad` builds the pack from the copy; delete Go list |
| T6   | radius  | Update comments and releases doc                    |
| T7   | contrib | Update pack header and README                       |
| T8   | -       | Post status on the issues                           |

---

### T1 (contrib): add PostgreSQL to the Kubernetes pack

- Why: closes the type gap in 1.7 (contrib#357).
- File: `recipe-packs/kubernetes/default.bicep`.
- Change: add

  ```bicep
  'Radius.Data/postgreSqlDatabases': {
    kind: 'bicep'
    source: 'ghcr.io/radius-project/kube-recipes/postgresqldatabases:latest'
  }
  ```

- Check first that the image tag exists, e.g. `oras manifest fetch ghcr.io/radius-project/kube-recipes/postgresqldatabases:latest`.
- Compare the final list with `GetCoreTypesRecipeInfo()` in radius: same 8 types, same image names, same `routes` parameters (`gatewayName: 'radius'`, `gatewayNamespace: 'radius-system'`).
- Done when: PR merged; pack lists the same 8 types as the Go list.

### T2 (contrib): release `recipe-pack/kubernetes/v0.3.0`

- Why: radius only moves a pin to a stable release (1.4).
- Workflow: contrib Actions -> "Release Recipe Pack" (`release-recipe-pack.yaml`). Inputs: `recipe_pack: kubernetes`, `bump: minor`, `dry_run: true` first.
- Read the dry-run summary: computed version (expect v0.3.0), detected changes, "Verify Recipe sources resolve" passes, bundle built.
- Run again with `dry_run: false`. This creates the tag, the GitHub Release (`recipe-pack-kubernetes-v0.3.0.tar.gz` + `checksums.txt`), and an attestation.
- Needs write access to contrib to run the workflow.
- Done when: release exists; `notify-radius.yaml` run for it succeeded.

### T3 (radius): review and merge the pin bump

- Why: makes v0.3.0 the pinned pack.
- Find the bot PR `chore(resource-types-contrib): updates` (branch `bot/update-resource-types`).
- Check in `deploy/manifest/defaults.yaml`: `recipePacks.kubernetes.ref` is the commit of `recipe-pack/kubernetes/v0.3.0` and `tag` is that tag. Verify with `git ls-remote https://github.com/radius-project/resource-types-contrib refs/tags/recipe-pack/kubernetes/v0.3.0`.
- Check CI is green, including "Verify resource types manifest".
- Done when: merged.

### T4 (radius): golden test of today's default pack

- Why: PR-B replaces where the pack comes from. This test proves the output does not change.
- File: `pkg/cli/recipepack/recipepack_test.go`.
- Note: the existing `Test_NewDefaultRecipePackResource` compares against `GetCoreTypesRecipeInfo()`, which PR-B deletes. The new test must write out the expected values literally.
- Cases:
  - edge build: 8 types, each source `ghcr.io/radius-project/kube-recipes/<name>:edge`, `routes` parameters, `kind: bicep`, `location: global`.
  - release build: same, tag = the namespace `ref` from `defaults.yaml`. Read the expected SHA via `defaults.ResourceTypePin` so the test does not break on every bot pin bump; write type list, image names, and parameters literally.
- Use the existing pattern in `Test_GetDefaultRecipePackDefinition_UsesEdgeTagForEdgeChannel` to force the edge/release branch.
- Run: `go test ./pkg/cli/recipepack/...`.
- Done when: merged before PR-B.

### PR-A (radius): sync copies and compiles the pinned pack

- Why: `rad` can only use a file that is in the repo at build time (D2).
- File: `build/scripts/sync-resource-types.sh`, function `copy_manifests`. The loop already fetches each pinned pack commit and checks `recipe-packs/<pack>` exists. For `kubernetes` only:
  1. Require `recipe-packs/kubernetes/default.bicep`; fail with a clear message if missing (D3).
  2. Compile to JSON.
  3. Remove `metadata._generator` so a Bicep version bump does not cause drift.
  4. Write `deploy/manifest/recipe-packs/kubernetes/default.json`.
- Compile details to settle in this PR:
  - The file uses `extension radius`, so compiling needs a `bicepconfig.json` for the Radius extension (`build/scripts/generate-bicepconfig.sh`).
  - Bicep version comes from `build/tools.yaml` (`make install-bicep`).
  - Add `make install-bicep` to `verify-resource-types-manifest.yaml` and to the bot job in `update-resource-types.yaml`.
- Drift check: add `deploy/manifest/recipe-packs/` to the `git status`/`git diff` paths in `verify-resource-types-manifest.yaml`.
- Run: `make sync-resource-types`, `make test-sync-resource-types`, `shellcheck`, `shfmt -i 4 -ci`.
- Done when: merged; committed `default.json` matches the pinned v0.3.0 pack; CI drift check covers it.

### T5 (radius): sync test cases for the pack copy

- Why: covers the failure paths PR-A added.
- File: `build/scripts/test-sync-resource-types.sh` (uses `assert_equal` and a `git` stub).
- Cases:
  - `recipe-packs/kubernetes/default.bicep` missing at the pinned commit -> sync fails with the PR-A message.
  - Pin moved to a newer release -> the copied JSON is replaced.
- Run: `make test-sync-resource-types`.
- Done when: merged.

### PR-B (radius): `rad` builds the pack from the copy

- Why: item 1.
- Steps:
  1. `deploy/manifest/embed.go`: add `recipe-packs/kubernetes/default.json` to `//go:embed` and add a path constant.
  2. `pkg/defaults`: loader that parses the JSON, finds the `Radius.Core/recipePacks` resource, and returns per resource type: recipe kind, image without tag, parameters. Load once in `init()` like the other lookups; on error, log and return empty.
  3. `pkg/cli/recipepack/recipepack.go`: `NewDefaultRecipePackResource()` uses the loader and sets each source to `<image>:` + `resolveRecipeTag(type, isEdge)`. Delete `GetCoreTypesRecipeInfo()` and the gateway parameter constants if unused.
  4. Decide behavior when the loader returns nothing (fail the command vs empty pack).
  5. Update or remove tests that call `GetCoreTypesRecipeInfo()`.
- T4 must pass unchanged.
- Run: `go test ./pkg/defaults/... ./pkg/cli/recipepack/... ./pkg/cli/cmd/...`, `make lint`.
- Done when: merged; Go list gone; T4 green.

### T6 (radius): update comments and releases doc

- Comments that say recipe packs are not copied:
  - `deploy/manifest/defaults.yaml` line 7
  - `build/resource-types.mk` line 36 and the `update-recipe-packs` target text
  - `build/scripts/sync-resource-types.sh` lines 18 and 649
- `docs/contributing/contributing-releases/README.md`: note that the Kubernetes pack is copied and compiled into `deploy/manifest/recipe-packs/`.
- Run `radius-markdown-lint` on changed Markdown.
- Done when: merged.

### T7 (contrib): update pack header and README

- `recipe-packs/kubernetes/default.bicep` header and `recipe-packs/kubernetes/README.md`: state that `rad` creates this same pack, and that manual registration should use the released file for the version matching the user's Radius install.
- Done when: merged.

### T8: post status

- Comment on #11959, #11835, contrib#290: what merged (links), what remains (#13210).
- Done when: posted.

---

## 6. Done for this plan

- contrib pack and `rad` pack: same resource types, images, parameters.
- `deploy/manifest/recipe-packs/kubernetes/default.json` matches the pinned release; CI fails on drift.
- `GetCoreTypesRecipeInfo()` deleted.
- Tags unchanged: release -> namespace commit SHA; edge -> `edge`.
