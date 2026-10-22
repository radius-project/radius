# Changelog

All notable changes to Radius are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Radius follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) with the version policy documented in the [release process](docs/contributing/contributing-releases/README.md).

## [Unreleased]

## [0.62.0-rc.1] - 2026-10-22

### Breaking changes

- Remove rad recipe register/unregister/list/show commands by @lakshmimsft in [#13236](https://github.com/radius-project/radius/pull/13236)
- Make rad env commands target Radius.Core without --preview by @lakshmimsft in [#13290](https://github.com/radius-project/radius/pull/13290)
- Pin chart images to full release versions by @DariuszPorowski in [#12953](https://github.com/radius-project/radius/pull/12953)
- Retire legacy paths and consume snapshots by @DariuszPorowski in [#12956](https://github.com/radius-project/radius/pull/12956)
- Make rad resource expose and logs target Radius.Compute/containers without --preview by @lakshmimsft in [#13319](https://github.com/radius-project/radius/pull/13319)
- Make rad app commands target Radius.Core without --preview by @lakshmimsft in [#13306](https://github.com/radius-project/radius/pull/13306)
- Make rad init and rad workspace create target Radius.Core without --preview by @lakshmimsft in [#13316](https://github.com/radius-project/radius/pull/13316)
- Make rad resource list target Radius.Core without --preview by @lakshmimsft in [#13317](https://github.com/radius-project/radius/pull/13317)
- Require gated promotion of external channel tags by @DariuszPorowski in [#13333](https://github.com/radius-project/radius/pull/13333)
- Remove unused --preview helpers and stale preview wording by @lakshmimsft in [#13326](https://github.com/radius-project/radius/pull/13326)

### Added

- Add conventional commit title generation by @DariuszPorowski in [#13138](https://github.com/radius-project/radius/pull/13138)
- Add flaky-test investigation skill and automation by @nicolejms in [#13190](https://github.com/radius-project/radius/pull/13190)
- Support S3 and Azure Blob backends with registered credentials by @lakshmimsft in [#13078](https://github.com/radius-project/radius/pull/13078)
- Publish release SBOMs by @DariuszPorowski in [#12869](https://github.com/radius-project/radius/pull/12869)
- [stack 1/3: default k8s recipe pack] sync and embed the pinned Kubernetes recipe pack by @nithyatsu in [#13248](https://github.com/radius-project/radius/pull/13248)
- [stack 2/3: default k8s recipe pack] build the default recipe pack from the embedded Kubernetes pack by @nithyatsu in [#13249](https://github.com/radius-project/radius/pull/13249)
- Always write bicepconfig.json in rad init --preview by @willdavsmith in [#13164](https://github.com/radius-project/radius/pull/13164)
- Add authorization mode setting (off/dryRun/enforce) by @sk593 in [#13212](https://github.com/radius-project/radius/pull/13212)
- Add authorization error codes by @sk593 in [#13213](https://github.com/radius-project/radius/pull/13213)
- Make the async worker dequeue interval configurable by @itpropro in [#13327](https://github.com/radius-project/radius/pull/13327)
- Fail functional tests on authz dry-run would-deny logs by @sk593 in [#13215](https://github.com/radius-project/radius/pull/13215)
- Add manual authz trust-boundary test kit by @sk593 in [#13216](https://github.com/radius-project/radius/pull/13216)
- Create the Deployment Engine tag after release approval by @DariuszPorowski in [#13384](https://github.com/radius-project/radius/pull/13384)

### Fixed

- Require Azure resource groups in Radius.Core environments by @Copilot in [#12495](https://github.com/radius-project/radius/pull/12495)
- Recognize ruleset-backed PR approvals by @DariuszPorowski in [#13111](https://github.com/radius-project/radius/pull/13111)
- Surface API response when CLI download test fails to parse version by @pujitha24 in [#12907](https://github.com/radius-project/radius/pull/12907)
- Bound bicep download with a timeout in the PowerShell installer by @sk593 in [#12852](https://github.com/radius-project/radius/pull/12852)
- Preserve change filtering in the merge queue by @DariuszPorowski in [#13139](https://github.com/radius-project/radius/pull/13139)
- Prevent MongoDB recipe scheduling timeout by @brooke-hamilton in [#13146](https://github.com/radius-project/radius/pull/13146)
- Validate default namespace in rad env create by @nicolejms in [#13156](https://github.com/radius-project/radius/pull/13156)
- Wrap long descriptions in rad resource-type show by @nicolejms in [#13158](https://github.com/radius-project/radius/pull/13158)
- Reconcile release tags and branches idempotently by @DariuszPorowski in [#12770](https://github.com/radius-project/radius/pull/12770)
- Share required Helm overrides across setup by @brooke-hamilton in [#13145](https://github.com/radius-project/radius/pull/13145)
- Prebuild rad for Windows process tests by @brooke-hamilton in [#13151](https://github.com/radius-project/radius/pull/13151)
- Retry transient registry failures when downloading Bicep recipes by @nicolejms in [#13185](https://github.com/radius-project/radius/pull/13185)
- Make resource type lookup case-insensitive in rad resource list/resource-type show by @nicolejms in [#13184](https://github.com/radius-project/radius/pull/13184)
- Retry all curl errors in tool install scripts by @nicolejms in [#13189](https://github.com/radius-project/radius/pull/13189)
- Mitigate flaky Install Radius timeout by waiting for Deployment Engine image in CI by @nicolejms in [#13191](https://github.com/radius-project/radius/pull/13191)
- Retry go mod download with HTTP/2 disabled to fix proxy.golang.org stream errors by @nicolejms in [#13199](https://github.com/radius-project/radius/pull/13199)
- Restore trusted change-detection helper before execution by @brooke-hamilton in [#13196](https://github.com/radius-project/radius/pull/13196)
- Refuse to persist an empty control-plane snapshot in rad shutdown by @pujitha24 in [#12849](https://github.com/radius-project/radius/pull/12849)
- Delete resource group contents in dependency order by @lakshmimsft in [#13155](https://github.com/radius-project/radius/pull/13155)
- Add validation for release metadata changes before trusted validator installation by @DariuszPorowski in [#13220](https://github.com/radius-project/radius/pull/13220)
- Coordinate managed-secret deletion in preview cascades by @brooke-hamilton in [#13193](https://github.com/radius-project/radius/pull/13193)
- Authenticate the nightly CLI release lookup by @AzureMike in [#13244](https://github.com/radius-project/radius/pull/13244)
- Honor reviewer-specific re-review handoffs by @DariuszPorowski in [#13255](https://github.com/radius-project/radius/pull/13255)
- Warn about Radius type version skew before deployment by @nicolejms in [#13228](https://github.com/radius-project/radius/pull/13228)
- Isolate the recipe pack missing-namespace test by @AzureMike in [#13257](https://github.com/radius-project/radius/pull/13257)
- Resolve API versions consistently for generic resource operations by @lakshmimsft in [#13232](https://github.com/radius-project/radius/pull/13232)
- Prevent missing base commit errors in fork PR change detection by @brooke-hamilton in [#13272](https://github.com/radius-project/radius/pull/13272)
- Retry managed secret delete while it is still updating by @nicolejms in [#13278](https://github.com/radius-project/radius/pull/13278)
- Retry transient API server errors when reading resources by @nicolejms in [#13277](https://github.com/radius-project/radius/pull/13277)
- Retry image pushes after unknown blob by @nicolejms in [#13288](https://github.com/radius-project/radius/pull/13288)
- Retry transient Helm chart downloads by @nicolejms in [#13286](https://github.com/radius-project/radius/pull/13286)
- Retry GitHub OIDC request timeouts before Azure login by @nicolejms in [#13287](https://github.com/radius-project/radius/pull/13287)
- Bound retries for Bicep downloads by @nicolejms in [#13291](https://github.com/radius-project/radius/pull/13291)
- Add provider alias placeholders before terraform get by @nicolejms in [#13300](https://github.com/radius-project/radius/pull/13300)
- Tolerate action SHA updates in release controller tests by @brooke-hamilton in [#13308](https://github.com/radius-project/radius/pull/13308)
- Install pinned tools without building tool-updater by @nicolejms in [#13313](https://github.com/radius-project/radius/pull/13313)
- Retry transient Terraform provider discovery failures by @nicolejms in [#13321](https://github.com/radius-project/radius/pull/13321)
- Standardize bounded external fetch retries by @nicolejms in [#13322](https://github.com/radius-project/radius/pull/13322)
- Hand off Copilot threads and author responses by @DariuszPorowski in [#13332](https://github.com/radius-project/radius/pull/13332)
- Report rejected deletes and handle rate limits by @nicolejms in [#13341](https://github.com/radius-project/radius/pull/13341)
- Delete Key Vault secrets through managed parent vaults by @nicolejms in [#13342](https://github.com/radius-project/radius/pull/13342)
- Preserve response metadata after retry timeout by @nicolejms in [#13353](https://github.com/radius-project/radius/pull/13353)
- Skip versions protected by download limits by @nicolejms in [#13363](https://github.com/radius-project/radius/pull/13363)
- Support digest-pinned Bicep recipe sources and keep registry ports in publish URLs by @AzureMike in [#13258](https://github.com/radius-project/radius/pull/13258)
- Select supported Azure API versions for resource deletion by @nicolejms in [#13370](https://github.com/radius-project/radius/pull/13370)
- Keep the Git server port-forward alive for Flux tests by @kachawla in [#13373](https://github.com/radius-project/radius/pull/13373)
- Fall back to parent types for Azure API version lookup by @nicolejms in [#13379](https://github.com/radius-project/radius/pull/13379)
- Avoid GraphQL node limit when collecting backports by @DariuszPorowski in [#13381](https://github.com/radius-project/radius/pull/13381)
- Skip last tagged version refusals by @nicolejms in [#13378](https://github.com/radius-project/radius/pull/13378)
- Skip k3d load balancer in Repo Radius GHCR state test by @kachawla in [#13386](https://github.com/radius-project/radius/pull/13386)

### Changed

- Remove unused code by @kachawla in [#13140](https://github.com/radius-project/radius/pull/13140)
- Reduce Make startup overhead by @brooke-hamilton in [#13285](https://github.com/radius-project/radius/pull/13285)
- Fold radinit/common back into radinit by @lakshmimsft in [#13325](https://github.com/radius-project/radius/pull/13325)
- Remove Spec Kit, migrate Radius skills, and relocate MCP config by @DariuszPorowski in [#13331](https://github.com/radius-project/radius/pull/13331)

### Dependencies

- Bump the go-deps group across 1 directory with 3 updates by @dependabot[bot] in [#13104](https://github.com/radius-project/radius/pull/13104)
- Bump the security group across 2 directories with 3 updates by @dependabot[bot] in [#13150](https://github.com/radius-project/radius/pull/13150)
- Bump the devcontainers group with 3 updates by @dependabot[bot] in [#13167](https://github.com/radius-project/radius/pull/13167)
- Bump the github-actions group across 1 directory with 3 updates by @dependabot[bot] in [#13171](https://github.com/radius-project/radius/pull/13171)
- Bump the javascript-deps group across 3 directories with 7 updates by @dependabot[bot] in [#13169](https://github.com/radius-project/radius/pull/13169)
- Bump the docker group across 5 directories with 2 updates by @dependabot[bot] in [#13170](https://github.com/radius-project/radius/pull/13170)
- Bump the go-deps group across 1 directory with 12 updates by @dependabot[bot] in [#13175](https://github.com/radius-project/radius/pull/13175)
- Bump the go-deps group across 2 directories with 16 updates by @dependabot[bot] in [#13240](https://github.com/radius-project/radius/pull/13240)
- Bump ghcr.io/devcontainers/features/docker-in-docker from 4.1.2 to 4.1.3 in the devcontainers group by @dependabot[bot] in [#13237](https://github.com/radius-project/radius/pull/13237)
- Bump the github-actions group across 2 directories with 3 updates by @dependabot[bot] in [#13241](https://github.com/radius-project/radius/pull/13241)
- Bump the docker group across 2 directories with 2 updates by @dependabot[bot] in [#13239](https://github.com/radius-project/radius/pull/13239)
- Bump the docker group across 2 directories with 2 updates by @dependabot[bot] in [#13302](https://github.com/radius-project/radius/pull/13302)
- Bump the go-deps group across 2 directories with 30 updates by @dependabot[bot] in [#13304](https://github.com/radius-project/radius/pull/13304)
- Bump the github-actions group across 2 directories with 8 updates by @dependabot[bot] in [#13305](https://github.com/radius-project/radius/pull/13305)
- Bump the javascript-deps group across 2 directories with 11 updates by @dependabot[bot] in [#13303](https://github.com/radius-project/radius/pull/13303)
- Bump the go-deps group across 2 directories with 18 updates by @dependabot[bot] in [#13365](https://github.com/radius-project/radius/pull/13365)
- Bump the github-actions group across 2 directories with 3 updates by @dependabot[bot] in [#13367](https://github.com/radius-project/radius/pull/13367)
- Bump the javascript-deps group across 3 directories with 4 updates by @dependabot[bot] in [#13366](https://github.com/radius-project/radius/pull/13366)
- Bump the docker group across 2 directories with 1 update by @dependabot[bot] in [#13364](https://github.com/radius-project/radius/pull/13364)

### Contributors

- @radius-resource-types[bot] in [#13314](https://github.com/radius-project/radius/pull/13314)
- @kachawla in [#13386](https://github.com/radius-project/radius/pull/13386)
- @DariuszPorowski in [#13385](https://github.com/radius-project/radius/pull/13385)
- @nicolejms in [#13378](https://github.com/radius-project/radius/pull/13378)
- @Copilot in [#13375](https://github.com/radius-project/radius/pull/13375)
- @radius-files-sync[bot] in [#13371](https://github.com/radius-project/radius/pull/13371)
- @sk593 in [#13216](https://github.com/radius-project/radius/pull/13216)
- @AzureMike in [#13258](https://github.com/radius-project/radius/pull/13258)
- @dependabot[bot] in [#13364](https://github.com/radius-project/radius/pull/13364)
- @lakshmimsft in [#13326](https://github.com/radius-project/radius/pull/13326)
- @itpropro in [#13327](https://github.com/radius-project/radius/pull/13327)
- @brooke-hamilton in [#13360](https://github.com/radius-project/radius/pull/13360)
- @allexistence in [#13318](https://github.com/radius-project/radius/pull/13318)
- @radius-dependabot-manager[bot] in [#13338](https://github.com/radius-project/radius/pull/13338)
- @willdavsmith in [#13164](https://github.com/radius-project/radius/pull/13164)
- @willtsai in [#13266](https://github.com/radius-project/radius/pull/13266)
- @nithyatsu in [#13250](https://github.com/radius-project/radius/pull/13250)
- @Reshrahim in [#13122](https://github.com/radius-project/radius/pull/13122)
- @pujitha24 in [#12849](https://github.com/radius-project/radius/pull/12849)
- @zachcasper in [#12662](https://github.com/radius-project/radius/pull/12662)

### New contributors

- @itpropro made their first contribution in [#13327](https://github.com/radius-project/radius/pull/13327)
- @allexistence made their first contribution in [#13318](https://github.com/radius-project/radius/pull/13318)


## [0.60.0] - 2026-08-19

### Added

- Added preview support for fully extensible, recipe-driven resource types in the `Radius.*` namespace.
- Added direct use of standard Bicep and Terraform modules as Radius Recipes.

### Changed

- Replaced SHA-1 with SHA-256 for internal resource identifiers, Terraform recipe state keys, ETags, and change-detection tokens while preserving compatibility with existing state.

For curated highlights, upgrade guidance, contributors, and the complete change list, see the [Radius v0.60.0 release notes](docs/release-notes/v0.60.0.md).

Release history before v0.60.0 remains available on [GitHub Releases](https://github.com/radius-project/radius/releases).

[Unreleased]: https://github.com/radius-project/radius/compare/v0.62.0-rc.1...HEAD
[0.62.0-rc.1]: https://github.com/radius-project/radius/compare/v0.61.1...v0.62.0-rc.1
[0.60.0]: https://github.com/radius-project/radius/releases/tag/v0.60.0
