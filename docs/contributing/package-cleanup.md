# Release-safe package cleanup

## Purpose

Use this guide to review and remove expired test container versions linked to `radius-project/radius`. The [shared engine](../../.github/scripts/package-cleanup.mjs) lists organization containers, checks each repository link, saves an inventory, and classifies versions as `keep`, `hold`, or `delete-candidate`. Package counts are not deletion approvals. Unknown and unlinked packages are held. Packages linked to other repositories are excluded. The engine never deletes a whole package.

The [scheduled/manual wrapper](../../.github/workflows/purge-old-images.yaml) now **only creates plans**. This intentionally replaces automatic deletion. The [apply wrapper](../../.github/workflows/package-cleanup-apply.yaml) uses the same [reusable workflow](../../.github/workflows/__package-cleanup.yaml) but requires separate activation, an approved plan, and a checked publication freeze. Merging this code does not enable deletion.

## Prerequisites

- For offline checks: Node.js 24, Bash, curl, jq, ShellCheck, and the repository build tools. No GitHub credential is needed for the tests.
- **MANUAL SETUP:** A repository administrator must confirm that the inventory account can see every linked package. A classic PAT needs `read:packages`, repository push visibility (including draft releases), and access to repository Actions metadata. Use `repo` scope for the repository/environment checks. The apply account also needs `delete:packages` and package admin rights. Confirm organization SSO authorization where required. GitHub App installation tokens cannot replace the package PAT.
- **MANUAL SETUP:** Confirm that `GH_RAD_CI_BOT_PAT`, if provided by the organization, is accessible to this repository. An absent repository-level secret does not prove that an organization secret is absent. The read-only `GITHUB_TOKEN` fallback may not have package access or draft-release visibility; failures are explicit.
- **MANUAL SETUP:** Create the `package-cleanup` environment before using apply. Add at least one required reviewer, enable **Prevent self-review**, disallow administrator bypass, and allow deployments from exactly the branch `main` with a custom branch rule. A tag rule or a "protected branches only" rule is not sufficient. Preflight checks these settings before the apply job can create/use an environment. If the API cannot read the rules, apply stops.
- **MANUAL SETUP:** Put `CLEANUP_PACKAGE_TOKEN` and `CLEANUP_PUBLICATION_FREEZE` in that environment. The apply wrapper must pass both secret names to the reusable workflow even when their values exist only in the protected environment. The environment values take precedence. Keep the deletion token restricted to this environment; do not use a repository-wide deletion credential as a substitute. A repository administrator must control changes to these settings and to trusted `main` code.

## Steps

### 1. Create the first inventory without deletion

After merge, run the plan workflow on `main`:

```sh
gh workflow run purge-old-images.yaml --ref main \
  -f profile=pr -f retention_days=7
```

Open that run and download `cleanup-plan-<run_attempt>`. The artifact lasts **30 days**, subject to repository retention limits. It contains `inventory.json`, `plan.json`, `plan.sha256`, and `report.json`. On error, inspect `error.json` and the partial inventory. No plan can be approved from an incomplete or failed run. Export evidence to an approved long-term store if it must outlive the artifact.

The collector paginates packages, versions, releases, and open PRs. It reads every visible release entry, including old releases, both `-rcN` and `-rc.N` candidates, drafts, and nonstandard development entries. It reads each linked version's published OCI manifest by digest and checks the content hash. Private or unavailable registry metadata causes holds; anonymous registry reads do not use the deletion token.

### 2. Resolve holds and review release evidence

**HOLD RESOLUTION:** The checked-in [evidence ledger](../../.github/package-cleanup-evidence.json) is deliberately incomplete and expired. Review it in a separate PR before activation. No test version becomes a candidate until all visible historical release entries and active references have reviewed evidence. Do not set the review flags just to clear a hold.

The engine seeds protection from checked-in [release baselines](../../.github/release-parity/baselines/) and binds the [release targets](../../.github/release-parity/targets.json), [version policy](../../.github/scripts/release-version.sh), and [OCI publisher](../../.github/scripts/release-oci-artifacts.sh) into the policy hash. Published version tags and dependency graphs reconstruct additional protection. These sources are useful evidence, not proof that historical coverage is complete.

For every entry in `inventory.json.releases`, add a ledger record with its exact `id`, `tag`, and `updatedAt`, a repository `source` URL for the reviewed evidence, and a nonempty `digests` array of verified OCI roots. Use `tag_name` and `updated_at` from the inventory for `tag` and `updatedAt`. Include image indexes, CLI OCI binaries, charts, recipes, and release artifacts across historical namespaces. Use published manifests, verified locks, baselines, and release build records where available. Historical releases do **not** all have a `release-manifest.json` asset. Missing records, moved tags, missing build records, or unknown artifact history remain holds. Do not guess a digest or assume that all prereleases are disposable.

For example, a release record has this shape (the digest below is a placeholder, not evidence):

```json
{
  "id": 123,
  "tag": "v0.60.0-rc3",
  "updatedAt": "2026-08-01T00:00:00Z",
  "source": "https://github.com/radius-project/radius/releases/tag/v0.60.0-rc3",
  "digests": ["sha256:0000000000000000000000000000000000000000000000000000000000000000"]
}
```

Add any other known release or active artifact roots to `protectedDigests`. Add active test tags to `activeTags`, including tests outside the repository and temporary test environments. Open PR tags are also protected automatically. For the old recipe profile, list only reviewed `dev/`, `radius/dev/`, or `test/testrecipes/test-bicep-recipes/` package paths in `testRecipePackages`. Set `review` to the evidence-review PR URL, set `expiresAt` to a reviewed expiry no more than seven days ahead, and set both review flags to `true` only after the review is complete. New or changed release entries, an expired review, or missing active-reference proof stop candidate selection.

Protection always wins over a test tag. Release/channel aliases protect their digests, copies of those digests, and platform descendants across linked namespaces. Signatures, SBOMs, attestations, CLI packages, charts, and known artifact subjects are retained. A retained parent retains shared children. An untagged version is **held**, not treated as unused. Missing manifests, unknown tags, mixed release/test tags, or incomplete dependency graphs never produce deletion candidates.

| Profile          | Candidate tag rule                                                                              | Package scope                                                                               |
|------------------|-------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------|
| `pr`             | `pr-N`, optional 7-40 character hex suffix; `0.42.42-pr-N`                                      | Linked `dev/` and `radius/dev/`                                                             |
| `functional`     | `pr-func<10 hex>` or `test-func<10 hex>-<run ID>-<attempt>`                                     | Linked `dev/` and `radius/dev/`, including runtime images such as `dev/ucpd`                |
| `test-recipes`   | PR/functional tags, `test-func<10 hex>-<run ID>-<attempt>`, or `test-<10 digit epoch>-<0..999>` | Exact reviewed paths under `dev/`, `radius/dev/`, or `test/testrecipes/test-bicep-recipes/` |
| `reviewed-tests` | Union of these rules                                                                            | Union of these scopes                                                                       |

Every tag on a candidate must match its profile. These forms come from [the cloud test workflow](../../.github/workflows/functional-test-cloud.yaml), [the recipe publisher](../../.github/scripts/publish-recipes.sh), and [the CLI recipe test](../../test/functional-portable/cli/noncloud/cli_test.go). Known Bicep module media types can enter test cleanup only at an exact reviewed recipe path; release recipes still have full release protection. Retention is at least seven days and uses both creation and update times. Legacy `radius/`, root production packages, `airg/`, `air-gapped-test/`, `rad/`, charts, and devcontainer packages are not enabled for deletion. Expanding into a shared release namespace needs a later code/policy review with complete release proof; there is no "ignore protection" input.

### 3. Establish a publication freeze

**MANUAL SETUP / HUMAN APPROVAL:** This implementation does not add a shared concurrency lock to publishers. Cleanup's concurrency group serializes cleanup only. Before the final plan and apply, an administrator must:

1. Record the current workflow states, then manually disable **all other repository workflows** except `purge-old-images.yaml`, `package-cleanup-apply.yaml`, and their reusable workflow `__package-cleanup.yaml`. Let active runs finish, or cancel them and confirm that they stopped. This includes tag releases, main builds, test publishers, reusable publishers, and manual workflows.
2. Stop and coordinate external publishers, sibling-repository publishers, manual GHCR pushes, and tag/alias changes for the full cleanup window. The repository workflow gate cannot lock external or manual publication. The reviewer must confirm this exclusion; without it, do not approve apply. Limit publication credentials during the window where practical.
3. Set the environment secret `CLEANUP_PUBLICATION_FREEZE` to a JSON lease with `repository`, the exact plan source `sha`, `externalPublishersStopped: true`, and `expiresAt` at most six hours ahead. Do not commit this operational secret. Do not renew the lease unless the freeze still holds.

```json
{
  "repository": "radius-project/radius",
  "sha": "<exact main commit SHA>",
  "externalPublishersStopped": true,
  "expiresAt": "<UTC timestamp within six hours>"
}
```

Apply checks workflow states, active runs, current `main`, environment rules, and the lease before inventory and before each DELETE. These checks enforce repository workflow exclusion but do not make revalidation atomic against an administrator who lifts the freeze or a person who uses external credentials. Maintain the human freeze until apply has stopped. If reliable exclusion is not possible, leave apply disabled.

### 4. Approve one fresh, unchanged plan

**HUMAN APPROVAL:** Create a new plan after the freeze starts. Review candidates, held versions, release roots, active references, and graph edges. The plan binds the repository, source commit, policy files, workflow run/attempt, package/version IDs, tags, digests, timestamps, reasons, inventory, and evidence. `plan.sha256` is the SHA256 of canonical JSON, not the raw file bytes.

Only after setup and review, an administrator may set the repository variable `PACKAGE_CLEANUP_ENABLE_APPLY` to the exact string `true`. This PR does not set it. Then dispatch:

```sh
gh workflow run package-cleanup-apply.yaml --ref main \
  -f plan_run_id=<reviewed-plan-run-id> \
  -f plan_sha256=<value-from-plan.sha256> \
  -f profile=pr -f retention_days=7 -f max_deletes=25
```

The plan must come from a successful `purge-old-images.yaml` run on this repository's `main`, with the same commit as apply. Preflight checks the existing environment rules and source run. The environment reviewer checks the exact run ID, hash, inputs, and freeze before approving. **Only one listed reviewer must approve**; preventing self-review requires another person when the dispatcher is a reviewer. A plan older than six hours, changed source/policy, or a changed hash is rejected. Artifact data cannot supply an API origin or a script.

Apply downloads only that run's checked attempt. It re-inventories and rechecks protection, releases, open PRs, and each version's metadata before DELETE. Changed records are skipped and require a new plan. Parent indexes are removed before candidate children. Refused/changed/absent parents keep their descendants. Shared children remain while any retained parent refers to them.

### 5. Review results, resume safely, and end the freeze

The default cap is 25 version delete operations; the maximum is 100 per apply. The engine allows 4,000 logical API requests by default, with a hard limit of 20,000. Each transport request has at most three HTTP attempts and a 60-second retry budget, so 25 version delete operations can send at most 75 DELETE requests. A registry read also needs a bounded anonymous token request. The cap is not a storage estimate.

Download `cleanup-apply-<run_attempt>`. `report.json` records kept, held, deleted, already absent, protected, and unfinished versions with reasons and counts. A protected download refusal warns and continues with independent candidates, but retains descendants. HTTP 404 is "already absent," never "deleted." A rate limit, cap, skipped candidate, or protected version produces a **partial** result and a failing job. Unknown errors are fatal. An interrupted run leaves durable checkpoints, but an in-flight request may have completed; check the registry on the next inventory.

**HOLD RESOLUTION:** To resume, keep or re-establish the freeze, fix the cause, generate a **new plan**, review its hash, and dispatch apply with that new run/hash. Do not edit an old plan, blindly rerun a failed deletion, or reuse an expired approval. Re-inventory finds versions already absent. Save the old and new reports as the audit trail.

After the last apply stops, remove activation (set `PACKAGE_CLEANUP_ENABLE_APPLY` to `false`), end the lease, and restore the exact prior workflow states. Confirm that publication can resume. Do not restore every workflow as enabled if it was disabled before the window.

## Verification

Run the offline checks from the repository root:

```sh
make test-purge-old-images
shellcheck -x .github/scripts/purge-old-images.sh .github/scripts/purge-old-images_test.sh
```

The Make target is part of `make test` in unit-test CI. It runs fake HTTP transport tests and Node tests for scope, pagination, protection, dependency order, exact download refusals, rate limits, plan validation, changes during apply, environment/freeze gates, and workflow contracts. It does not delete a real package.

Before approving live apply, check the actual artifacts and environment settings. A successful plan is **not** a successful cleanup. An apply with `partial` or `failed` status is not complete. No report claims storage reclaimed: manifests can share layers.

## Troubleshooting

- **MANUAL SETUP: HTTP 403, missing token, or draft visibility failure.** Ask the credential owner to check organization-secret access, classic PAT scopes, SSO, repository push visibility, metadata access, and package admin rights. Do not print tokens or change your interactive token to test deletion.
- **HOLD RESOLUTION: No candidates.** Read each hold reason and the evidence ledger. The default ledger intentionally holds test versions. Resolve historical/active-reference evidence, not just the tag match. Private registry metadata, incomplete graphs, and unknown history remain holds.
- **HUMAN APPROVAL: Preflight fails or apply is skipped.** Confirm activation, exact `main` source, trusted successful plan run, environment reviewers/rules, disabled publishers, stopped runs, and freeze lease. Do not bypass a failed check.
- **SUPPORT: Download-protected version.** Only HTTP 400 with the exact JSON message `Publicly visible package versions with more than 5000 downloads cannot be deleted. Contact GitHub support for further assistance.` is continued. Open a [GitHub Support request](https://support.github.com/contact) with the organization, package path, version ID/digest, HTTP response, and run/report link. A maintainer must review any Support-assisted removal; it is not an automatic retry path.
- **SUPPORT: Other HTTP 400, 401, 403, 409, or 422.** Treat the error as a failure. Inspect the saved evidence and API diagnostic. Resolve access, last-tag, conflict, or other policy errors before a newly approved plan. There is no blanket ignore rule.
- **Limit: Production/legacy cleanup.** It is not enabled, even with complete test evidence. This code does not certify missing historical records or coordinate external publishers automatically. Keep these groups on hold until a separate reviewed change can prove safe removal.
