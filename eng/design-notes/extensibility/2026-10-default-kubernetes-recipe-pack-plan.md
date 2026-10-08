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

Example: the Kubernetes pack is released as v0.3.0 (step C2).

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
- Consequence: after P0 the new pin is recorded but unused. `rad` starts using it only after PR 3.

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

## 5. Work plan

The radius work is a stack of 4 PRs. Each PR targets the branch of the PR below it, so each diff stays small and reviewable. The contrib work is in a different repo and cannot be part of the stack; it is a prerequisite.

```text
contrib:  C1 add PostgreSQL -> C2 release v0.3.0 -> (bot) pin bump PR merged to radius main
radius:   main <- PR 1 <- PR 2 <- PR 3 <- PR 4
contrib:  C3 README (after PR 3 merges)
```

| Order | Step | Repo    | Base branch | Summary                                                  | Merge gate                |
|-------|------|---------|-------------|----------------------------------------------------------|---------------------------|
| 1     | C1   | contrib | `main`      | Add PostgreSQL to the Kubernetes pack                    | -                         |
| 2     | C2   | contrib | -           | Release `recipe-pack/kubernetes/v0.3.0`                  | C1 merged                 |
| 3     | P0   | radius  | `main`      | Review and merge the bot pin bump PR                     | C2 released               |
| 4     | PR 1 | radius  | `main`      | Sync compiles and copies the pinned pack; tests; CI      | P0 merged (pin at v0.3.0) |
| 5     | PR 2 | radius  | PR 1        | Embed the copied pack and add a loader in `pkg/defaults` | -                         |
| 6     | PR 3 | radius  | PR 2        | `rad` builds the pack from the loader; delete Go list    | -                         |
| 7     | PR 4 | radius  | PR 3        | Releases doc                                             | -                         |
| 8     | C3   | contrib | `main`      | Update pack header and README                            | PR 3 merged               |
| 9     | S    | -       | -           | Post status on the issues                                | all merged                |

Do the steps in the order shown. Steps 1 to 3 (contrib, then the pin bump) are the critical path, because PR 1 cannot merge until the pin is at v0.3.0.

Stack rules:

- Title each stacked PR `<type>(<scope>): [stack N/4: default k8s recipe pack] <subject>`. The label goes after `<type>(<scope>):` because the required Conventional Commit title check rejects anything before the type.
- Open PR 1 to PR 4 as soon as each is ready, each based on the previous branch. Mark PRs above the bottom one as draft until the one below merges.
- Merge bottom-up. After a PR merges (squash), rebase the next branch onto `main` and retarget its PR to `main`.
- PR 1 can be written now but must not merge until P0 is on `main`, because D3 makes the sync fail if `default.bicep` is missing at the pinned commit (v0.2.0 has `default-recipepack.bicep`). Rebase the stack onto `main` after P0 merges.
- PR 2 adds code with no caller, so it is safe to merge on its own. PR 3 is the only PR that changes `rad` behavior.

---

### C1 (contrib): add PostgreSQL to the Kubernetes pack

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

### C2 (contrib): release `recipe-pack/kubernetes/v0.3.0`

- Why: radius only moves a pin to a stable release (1.4).
- Workflow: contrib Actions -> "Release Recipe Pack" (`release-recipe-pack.yaml`). Inputs: `recipe_pack: kubernetes`, `bump: minor`, `dry_run: true` first.
- Read the dry-run summary: computed version (expect v0.3.0), detected changes, "Verify Recipe sources resolve" passes, bundle built.
- Run again with `dry_run: false`. This creates the tag, the GitHub Release (`recipe-pack-kubernetes-v0.3.0.tar.gz` + `checksums.txt`), and an attestation.
- Needs write access to contrib to run the workflow.
- Done when: release exists; `notify-radius.yaml` run for it succeeded.

### P0 (radius): review and merge the pin bump

- Why: makes v0.3.0 the pinned pack.
- Find the bot PR `chore(resource-types-contrib): updates` (branch `bot/update-resource-types`).
- Check in `deploy/manifest/defaults.yaml`: `recipePacks.kubernetes.ref` is the commit of `recipe-pack/kubernetes/v0.3.0` and `tag` is that tag. Verify with `git ls-remote https://github.com/radius-project/resource-types-contrib refs/tags/recipe-pack/kubernetes/v0.3.0`.
- Check CI is green, including "Verify resource types manifest".
- After this merges, the pin is recorded but still unused (1.5).
- Done when: merged.

### PR 1 (radius, base `main`): sync compiles and copies the pinned pack

- Why: `rad` can only use a file that is in the repo at build time (D2).
- Script: `build/scripts/sync-resource-types.sh`.
  1. Add a function (e.g. `copy_kubernetes_recipe_pack`) that, for the `kubernetes` pin only:
     - fetches the pinned commit (reuse the fetch already done in `copy_manifests`);
     - requires `recipe-packs/kubernetes/default.bicep`, and fails with a clear message if missing (D3);
     - compiles it to JSON;
     - removes `metadata._generator` so a Bicep version bump does not cause drift;
     - writes `deploy/manifest/recipe-packs/kubernetes/default.json`.
  2. Call it from every mode that can change or verify the pack pin:
     - default mode (`make sync-resource-types`) and `--update-all`, through `copy_manifests`;
     - `--update-recipe-packs` (`make update-recipe-packs`). Today this mode returns early, before `copy_manifests` (around lines 649-659 of `main`). Without this, `make update-recipe-packs RECIPE_PACKS_NAME=kubernetes` would move the pin and leave `default.json` stale.
- Bicep tooling, so local and CI output match:
  - The pack uses `extension radius`, so compiling needs a `bicepconfig.json` for the Radius extension (`build/scripts/generate-bicepconfig.sh`).
  - Use the Bicep version pinned in `build/tools.yaml`. The script fails with a clear message if `bicep` is missing or its version differs, and tells the user to run `make install-bicep`.
  - Add `make install-bicep` to `verify-resource-types-manifest.yaml` and to the bot job in `update-resource-types.yaml`.
- Drift check: add `deploy/manifest/recipe-packs/` to the `git status`/`git diff` paths in `verify-resource-types-manifest.yaml`.
- Update the "not vendored" comments in the same PR, since this PR makes them false:
  - `deploy/manifest/defaults.yaml` line 7
  - `build/resource-types.mk` line 36 and the `update-recipe-packs` target text
  - `build/scripts/sync-resource-types.sh` lines 18 and 649
- Tests in `build/scripts/test-sync-resource-types.sh` (uses `assert_equal` and a `git` stub):
  - `default.bicep` missing at the pinned commit -> sync fails with the new message.
  - Pin moved to a newer release -> the copied JSON is replaced.
  - `--update-recipe-packs` with `RECIPE_PACKS_NAME=kubernetes` -> pin and `default.json` both updated.
  - `--update-recipe-packs` for an Azure pack -> `default.json` unchanged.
  - Stub `bicep` the same way `git` is stubbed, so tests do not need the real binary.
- Run: `make install-bicep`, `make sync-resource-types`, `make test-sync-resource-types`, `shellcheck`, `shfmt -i 4 -ci`.
- Done when: merged; committed `default.json` matches the pinned v0.3.0 pack; CI drift check covers it.

### PR 2 (radius, base PR 1): embed the pack and add a loader

- Why: makes the copied pack available to Go code, without changing behavior yet.
- Steps:
  1. `deploy/manifest/embed.go`: add `recipe-packs/kubernetes/default.json` to `//go:embed` and add a path constant.
  2. `pkg/defaults`: loader that parses the JSON, finds the `Radius.Core/recipePacks` resource, and returns per resource type: recipe kind, image without tag, parameters. Load once in `init()` like the other lookups; on error, log and return empty.
  3. Unit tests for the loader: parses the embedded file (8 types); strips the tag from `image:tag`; bad JSON returns empty and an error.
- Run: `go test ./pkg/defaults/... ./deploy/manifest/...`, `make lint`.
- Done when: merged.

### PR 3 (radius, base PR 2): `rad` builds the pack from the loader

- Why: item 1.
- Steps:
  1. `pkg/cli/recipepack/recipepack.go`: `NewDefaultRecipePackResource()` uses the loader and sets each source to `<image>:` + `resolveRecipeTag(type, isEdge)`.
  2. Delete `GetCoreTypesRecipeInfo()` and the gateway parameter constants if unused.
  3. Decide behavior when the loader returns nothing (fail the command vs empty pack).
  4. Update or remove tests that call `GetCoreTypesRecipeInfo()`.
- Tests: for edge and release builds, the pack `rad` builds has exactly the recipe types, images and parameters in the embedded `default.json`, with the tag from `resolveRecipeTag` (edge -> `edge`, release -> namespace pin `ref`). Do not hard-code the type list; it changes with the synced pack. Add an unexported `isEdge` parameter (as `resolveRecipeTag` has) so the release case is testable.
- Run: `go test ./pkg/defaults/... ./pkg/cli/recipepack/... ./pkg/cli/cmd/...`, `make lint`.
- Done when: merged; Go list gone.

### PR 4 (radius, base PR 3): releases doc

- `docs/contributing/contributing-releases/README.md`: note that the Kubernetes pack is copied and compiled into `deploy/manifest/recipe-packs/`, that `make install-bicep` is needed for the sync, and that `rad` builds the default pack from that file.
- Run `radius-markdown-lint` on changed Markdown.
- Done when: merged.

### C3 (contrib): update pack header and README

- `recipe-packs/kubernetes/default.bicep` header and `recipe-packs/kubernetes/README.md`: state that `rad` creates this same pack, and that manual registration should use the released file for the version matching the user's Radius install.
- Done when: merged.

### S: post status

- Comment on #11959, #11835, contrib#290: what merged (links), what remains (#13210).
- Done when: posted.

---

## 6. Done for this plan

- contrib pack and `rad` pack: same resource types, images, parameters.
- `deploy/manifest/recipe-packs/kubernetes/default.json` matches the pinned release; CI fails on drift.
- `GetCoreTypesRecipeInfo()` deleted.
- Tags unchanged: release -> namespace commit SHA; edge -> `edge`.
