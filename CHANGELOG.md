# Changelog

All notable changes to Radius are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Radius follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) with the version policy documented in the [release process](docs/contributing/contributing-releases/README.md).

## [Unreleased]

## [0.62.0] - 2026-10-27

### Breaking changes

- Remove legacy Azure Container Instances support (#13293) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13483](https://github.com/radius-project/radius/pull/13483)
- Remove unused --preview helpers and stale preview wording by [@lakshmimsft](https://github.com/lakshmimsft) in [#13326](https://github.com/radius-project/radius/pull/13326)
- Require gated promotion of external channel tags by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13333](https://github.com/radius-project/radius/pull/13333)
- Make rad resource list target Radius.Core without --preview by [@lakshmimsft](https://github.com/lakshmimsft) in [#13317](https://github.com/radius-project/radius/pull/13317)
- Make rad init and rad workspace create target Radius.Core without --preview by [@lakshmimsft](https://github.com/lakshmimsft) in [#13316](https://github.com/radius-project/radius/pull/13316)
- Make rad app commands target Radius.Core without --preview by [@lakshmimsft](https://github.com/lakshmimsft) in [#13306](https://github.com/radius-project/radius/pull/13306)
- Make rad resource expose and logs target Radius.Compute/containers without --preview by [@lakshmimsft](https://github.com/lakshmimsft) in [#13319](https://github.com/radius-project/radius/pull/13319)
- Retire legacy paths and consume snapshots by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12956](https://github.com/radius-project/radius/pull/12956)
- Pin chart images to full release versions by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12953](https://github.com/radius-project/radius/pull/12953)
- Make rad env commands target Radius.Core without --preview by [@lakshmimsft](https://github.com/lakshmimsft) in [#13290](https://github.com/radius-project/radius/pull/13290)
- Remove rad recipe register/unregister/list/show commands by [@lakshmimsft](https://github.com/lakshmimsft) in [#13236](https://github.com/radius-project/radius/pull/13236)

### Added

- Add expression evaluator for direct module recipe parameters (#13297) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13486](https://github.com/radius-project/radius/pull/13486)
- Create the Deployment Engine tag after release approval by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13384](https://github.com/radius-project/radius/pull/13384)
- Add manual authz trust-boundary test kit by [@sk593](https://github.com/sk593) in [#13216](https://github.com/radius-project/radius/pull/13216)
- Fail functional tests on authz dry-run would-deny logs by [@sk593](https://github.com/sk593) in [#13215](https://github.com/radius-project/radius/pull/13215)
- Make the async worker dequeue interval configurable by [@itpropro](https://github.com/itpropro) in [#13327](https://github.com/radius-project/radius/pull/13327)
- Add authorization error codes by [@sk593](https://github.com/sk593) in [#13213](https://github.com/radius-project/radius/pull/13213)
- Add authorization mode setting (off/dryRun/enforce) by [@sk593](https://github.com/sk593) in [#13212](https://github.com/radius-project/radius/pull/13212)
- Always write bicepconfig.json in rad init --preview by [@willdavsmith](https://github.com/willdavsmith) in [#13164](https://github.com/radius-project/radius/pull/13164)
- [stack 2/3: default k8s recipe pack] build the default recipe pack from the embedded Kubernetes pack by [@nithyatsu](https://github.com/nithyatsu) in [#13249](https://github.com/radius-project/radius/pull/13249)
- [stack 1/3: default k8s recipe pack] sync and embed the pinned Kubernetes recipe pack by [@nithyatsu](https://github.com/nithyatsu) in [#13248](https://github.com/radius-project/radius/pull/13248)
- Publish release SBOMs by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12869](https://github.com/radius-project/radius/pull/12869)
- Support S3 and Azure Blob backends with registered credentials by [@lakshmimsft](https://github.com/lakshmimsft) in [#13078](https://github.com/radius-project/radius/pull/13078)
- Add flaky-test investigation skill and automation by [@nicolejms](https://github.com/nicolejms) in [#13190](https://github.com/radius-project/radius/pull/13190)
- Add conventional commit title generation by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13138](https://github.com/radius-project/radius/pull/13138)

### Fixed

- Honor KUBECONFIG when selecting the Kubernetes cluster (#13488) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13517](https://github.com/radius-project/radius/pull/13517)
- Dispatch fresh sample tests when resuming failures (#13512) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13514](https://github.com/radius-project/radius/pull/13514)
- Align make install destinations with CLI installers (#13511) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13513](https://github.com/radius-project/radius/pull/13513)
- Stop rad upgrade from failing on field manager conflicts (#13474) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13509](https://github.com/radius-project/radius/pull/13509)
- Update release-backport action to specific commit (#13504) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13505](https://github.com/radius-project/radius/pull/13505)
- Recover upmerges and share backport automation (#13498) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13503](https://github.com/radius-project/radius/pull/13503)
- Match Bicep registry credentials to the registry Radius pulls from (#13489) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13501](https://github.com/radius-project/radius/pull/13501)
- Stop retrying permanent Bicep delete errors (#13451) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13487](https://github.com/radius-project/radius/pull/13487)
- Sort release history by revision (#13435) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13484](https://github.com/radius-project/radius/pull/13484)
- Include unchanged changelog in plan snapshots (#13445) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13447](https://github.com/radius-project/radius/pull/13447)
- Include maintenance commits in changelogs (#13444) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13446](https://github.com/radius-project/radius/pull/13446)
- Align backports, changelogs, and sibling releases (#13433) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13441](https://github.com/radius-project/radius/pull/13441)
- Allow reviewed root changelog edits (#13434) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13440](https://github.com/radius-project/radius/pull/13440)
- Authenticate docs and samples coordination (#13431) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13438](https://github.com/radius-project/radius/pull/13438)
- Pin changelog rendering to the product commit (#13425) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13426](https://github.com/radius-project/radius/pull/13426)
- Preserve encryption Secret state during upgrades (#13413) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13424](https://github.com/radius-project/radius/pull/13424)
- Cache AWS DescribeType results and log AWS errors returned as 500 (#13412) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13423](https://github.com/radius-project/radius/pull/13423)
- Stop running Test_GetVersion subtests in parallel (#13411) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13422](https://github.com/radius-project/radius/pull/13422)
- Handle delayed GitHub release visibility (#13409) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13421](https://github.com/radius-project/radius/pull/13421)
- Wait for Azure blob read access before Terraform deployment by [@nicolejms](https://github.com/nicolejms) in [#13383](https://github.com/radius-project/radius/pull/13383)
- Retain legacy changes and deduplicate release notes by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13399](https://github.com/radius-project/radius/pull/13399)
- Link release-note mentions and compact contributors by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13392](https://github.com/radius-project/radius/pull/13392)
- Filter release-metadata-only PR checks by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13396](https://github.com/radius-project/radius/pull/13396)
- Skip k3d load balancer in Repo Radius GHCR state test by [@kachawla](https://github.com/kachawla) in [#13386](https://github.com/radius-project/radius/pull/13386)
- Skip last tagged version refusals by [@nicolejms](https://github.com/nicolejms) in [#13378](https://github.com/radius-project/radius/pull/13378)
- Avoid GraphQL node limit when collecting backports by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13381](https://github.com/radius-project/radius/pull/13381)
- Fall back to parent types for Azure API version lookup by [@nicolejms](https://github.com/nicolejms) in [#13379](https://github.com/radius-project/radius/pull/13379)
- Keep the Git server port-forward alive for Flux tests by [@kachawla](https://github.com/kachawla) in [#13373](https://github.com/radius-project/radius/pull/13373)
- Select supported Azure API versions for resource deletion by [@nicolejms](https://github.com/nicolejms) in [#13370](https://github.com/radius-project/radius/pull/13370)
- Support digest-pinned Bicep recipe sources and keep registry ports in publish URLs by [@AzureMike](https://github.com/AzureMike) in [#13258](https://github.com/radius-project/radius/pull/13258)
- Skip versions protected by download limits by [@nicolejms](https://github.com/nicolejms) in [#13363](https://github.com/radius-project/radius/pull/13363)
- Preserve response metadata after retry timeout by [@nicolejms](https://github.com/nicolejms) in [#13353](https://github.com/radius-project/radius/pull/13353)
- Delete Key Vault secrets through managed parent vaults by [@nicolejms](https://github.com/nicolejms) in [#13342](https://github.com/radius-project/radius/pull/13342)
- Report rejected deletes and handle rate limits by [@nicolejms](https://github.com/nicolejms) in [#13341](https://github.com/radius-project/radius/pull/13341)
- Hand off Copilot threads and author responses by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13332](https://github.com/radius-project/radius/pull/13332)
- Standardize bounded external fetch retries by [@nicolejms](https://github.com/nicolejms) in [#13322](https://github.com/radius-project/radius/pull/13322)
- Retry transient Terraform provider discovery failures by [@nicolejms](https://github.com/nicolejms) in [#13321](https://github.com/radius-project/radius/pull/13321)
- Install pinned tools without building tool-updater by [@nicolejms](https://github.com/nicolejms) in [#13313](https://github.com/radius-project/radius/pull/13313)
- Tolerate action SHA updates in release controller tests by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13308](https://github.com/radius-project/radius/pull/13308)
- Add provider alias placeholders before terraform get by [@nicolejms](https://github.com/nicolejms) in [#13300](https://github.com/radius-project/radius/pull/13300)
- Bound retries for Bicep downloads by [@nicolejms](https://github.com/nicolejms) in [#13291](https://github.com/radius-project/radius/pull/13291)
- Retry GitHub OIDC request timeouts before Azure login by [@nicolejms](https://github.com/nicolejms) in [#13287](https://github.com/radius-project/radius/pull/13287)
- Retry transient Helm chart downloads by [@nicolejms](https://github.com/nicolejms) in [#13286](https://github.com/radius-project/radius/pull/13286)
- Retry image pushes after unknown blob by [@nicolejms](https://github.com/nicolejms) in [#13288](https://github.com/radius-project/radius/pull/13288)
- Retry transient API server errors when reading resources by [@nicolejms](https://github.com/nicolejms) in [#13277](https://github.com/radius-project/radius/pull/13277)
- Retry managed secret delete while it is still updating by [@nicolejms](https://github.com/nicolejms) in [#13278](https://github.com/radius-project/radius/pull/13278)
- Prevent missing base commit errors in fork PR change detection by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13272](https://github.com/radius-project/radius/pull/13272)
- Resolve API versions consistently for generic resource operations by [@lakshmimsft](https://github.com/lakshmimsft) in [#13232](https://github.com/radius-project/radius/pull/13232)
- Isolate the recipe pack missing-namespace test by [@AzureMike](https://github.com/AzureMike) in [#13257](https://github.com/radius-project/radius/pull/13257)
- Warn about Radius type version skew before deployment by [@nicolejms](https://github.com/nicolejms) in [#13228](https://github.com/radius-project/radius/pull/13228)
- Honor reviewer-specific re-review handoffs by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13255](https://github.com/radius-project/radius/pull/13255)
- Authenticate the nightly CLI release lookup by [@AzureMike](https://github.com/AzureMike) in [#13244](https://github.com/radius-project/radius/pull/13244)
- Coordinate managed-secret deletion in preview cascades by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13193](https://github.com/radius-project/radius/pull/13193)
- Add validation for release metadata changes before trusted validator installation by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13220](https://github.com/radius-project/radius/pull/13220)
- Delete resource group contents in dependency order by [@lakshmimsft](https://github.com/lakshmimsft) in [#13155](https://github.com/radius-project/radius/pull/13155)
- Refuse to persist an empty control-plane snapshot in rad shutdown by [@pujitha24](https://github.com/pujitha24) in [#12849](https://github.com/radius-project/radius/pull/12849)
- Restore trusted change-detection helper before execution by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13196](https://github.com/radius-project/radius/pull/13196)
- Retry go mod download with HTTP/2 disabled to fix proxy.golang.org stream errors by [@nicolejms](https://github.com/nicolejms) in [#13199](https://github.com/radius-project/radius/pull/13199)
- Mitigate flaky Install Radius timeout by waiting for Deployment Engine image in CI by [@nicolejms](https://github.com/nicolejms) in [#13191](https://github.com/radius-project/radius/pull/13191)
- Retry all curl errors in tool install scripts by [@nicolejms](https://github.com/nicolejms) in [#13189](https://github.com/radius-project/radius/pull/13189)
- Make resource type lookup case-insensitive in rad resource list/resource-type show by [@nicolejms](https://github.com/nicolejms) in [#13184](https://github.com/radius-project/radius/pull/13184)
- Retry transient registry failures when downloading Bicep recipes by [@nicolejms](https://github.com/nicolejms) in [#13185](https://github.com/radius-project/radius/pull/13185)
- Prebuild rad for Windows process tests by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13151](https://github.com/radius-project/radius/pull/13151)
- Share required Helm overrides across setup by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13145](https://github.com/radius-project/radius/pull/13145)
- Reconcile release tags and branches idempotently by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12770](https://github.com/radius-project/radius/pull/12770)
- Wrap long descriptions in rad resource-type show by [@nicolejms](https://github.com/nicolejms) in [#13158](https://github.com/radius-project/radius/pull/13158)
- Validate default namespace in rad env create by [@nicolejms](https://github.com/nicolejms) in [#13156](https://github.com/radius-project/radius/pull/13156)
- Prevent MongoDB recipe scheduling timeout by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13146](https://github.com/radius-project/radius/pull/13146)
- Preserve change filtering in the merge queue by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13139](https://github.com/radius-project/radius/pull/13139)
- Bound bicep download with a timeout in the PowerShell installer by [@sk593](https://github.com/sk593) in [#12852](https://github.com/radius-project/radius/pull/12852)
- Surface API response when CLI download test fails to parse version by [@pujitha24](https://github.com/pujitha24) in [#12907](https://github.com/radius-project/radius/pull/12907)
- Recognize ruleset-backed PR approvals by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13111](https://github.com/radius-project/radius/pull/13111)
- Require Azure resource groups in Radius.Core environments by [@Copilot](https://github.com/Copilot) in [#12495](https://github.com/radius-project/radius/pull/12495)

### Changed

- Remove Spec Kit, migrate Radius skills, and relocate MCP config by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13331](https://github.com/radius-project/radius/pull/13331)
- Fold radinit/common back into radinit by [@lakshmimsft](https://github.com/lakshmimsft) in [#13325](https://github.com/radius-project/radius/pull/13325)
- Reduce Make startup overhead by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13285](https://github.com/radius-project/radius/pull/13285)
- Remove unused code by [@kachawla](https://github.com/kachawla) in [#13140](https://github.com/radius-project/radius/pull/13140)

### Dependencies

- Update Azure SDK dependencies and refactor imports (#13419) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13439](https://github.com/radius-project/radius/pull/13439)
- Remove unused oav dev dependency (#13408) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13420](https://github.com/radius-project/radius/pull/13420)
- Update npm dependencies with open security alerts by [@kachawla](https://github.com/kachawla) in [#13402](https://github.com/radius-project/radius/pull/13402)
- Bump @azure-tools/typespec-autorest from 0.73.0 to 0.73.1 in /typespec in the javascript-deps group across 1 directory by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13394](https://github.com/radius-project/radius/pull/13394)
- Bump the github-actions group across 1 directory with 5 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13397](https://github.com/radius-project/radius/pull/13397)
- Bump the go-deps group across 2 directories with 8 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13393](https://github.com/radius-project/radius/pull/13393)
- Bump the docker group across 3 directories with 1 update by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13395](https://github.com/radius-project/radius/pull/13395)
- Bump the docker group across 2 directories with 1 update by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13364](https://github.com/radius-project/radius/pull/13364)
- Bump the javascript-deps group across 3 directories with 4 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13366](https://github.com/radius-project/radius/pull/13366)
- Bump the github-actions group across 2 directories with 3 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13367](https://github.com/radius-project/radius/pull/13367)
- Bump the go-deps group across 2 directories with 18 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13365](https://github.com/radius-project/radius/pull/13365)
- Bump the javascript-deps group across 2 directories with 11 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13303](https://github.com/radius-project/radius/pull/13303)
- Bump the github-actions group across 2 directories with 8 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13305](https://github.com/radius-project/radius/pull/13305)
- Bump the go-deps group across 2 directories with 30 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13304](https://github.com/radius-project/radius/pull/13304)
- Bump the docker group across 2 directories with 2 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13302](https://github.com/radius-project/radius/pull/13302)
- Bump the docker group across 2 directories with 2 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13239](https://github.com/radius-project/radius/pull/13239)
- Bump the github-actions group across 2 directories with 3 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13241](https://github.com/radius-project/radius/pull/13241)
- Bump ghcr.io/devcontainers/features/docker-in-docker from 4.1.2 to 4.1.3 in the devcontainers group by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13237](https://github.com/radius-project/radius/pull/13237)
- Bump the go-deps group across 2 directories with 16 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13240](https://github.com/radius-project/radius/pull/13240)
- Bump the go-deps group across 1 directory with 12 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13175](https://github.com/radius-project/radius/pull/13175)
- Bump the docker group across 5 directories with 2 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13170](https://github.com/radius-project/radius/pull/13170)
- Bump the javascript-deps group across 3 directories with 7 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13169](https://github.com/radius-project/radius/pull/13169)
- Bump the github-actions group across 1 directory with 3 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13171](https://github.com/radius-project/radius/pull/13171)
- Bump the devcontainers group with 3 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13167](https://github.com/radius-project/radius/pull/13167)
- Bump the security group across 2 directories with 3 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13150](https://github.com/radius-project/radius/pull/13150)
- Bump the go-deps group across 1 directory with 3 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13104](https://github.com/radius-project/radius/pull/13104)

### Others

- Prepare v0.62.0-rc.6 (#13518) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13519](https://github.com/radius-project/radius/pull/13519)
- Clarify fresh-head upmerge retries (#13508) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13515](https://github.com/radius-project/radius/pull/13515)
- Prepare v0.62.0-rc.5 (#13506) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13507](https://github.com/radius-project/radius/pull/13507)
- Prepare v0.62.0-rc.4 (#13492) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13493](https://github.com/radius-project/radius/pull/13493)
- Point design note contributors to eng/design-notes (#13456) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13490](https://github.com/radius-project/radius/pull/13490)
- Add functional spec for ECS and Azure Container Apps core resource mappings (#13233) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13485](https://github.com/radius-project/radius/pull/13485)
- Prepare v0.62.0-rc.3 (#13449) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13450](https://github.com/radius-project/radius/pull/13450)
- Bump versions (#13442) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13448](https://github.com/radius-project/radius/pull/13448)
- Add a concise release runbook (#13390) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13437](https://github.com/radius-project/radius/pull/13437)
- Prepare v0.62.0-rc.2 (#13428) by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13429](https://github.com/radius-project/radius/pull/13429)
- Prepare v0.62.0-rc.1 by [@radius-release[bot]](https://github.com/apps/radius-release) in [#13403](https://github.com/radius-project/radius/pull/13403)
- Synced file(s) with radius-project/.github by [@radius-files-sync[bot]](https://github.com/apps/radius-files-sync) in [#13405](https://github.com/radius-project/radius/pull/13405)
- Clarify bug and private vulnerability reporting by [@vinayada1](https://github.com/vinayada1) in [#13404](https://github.com/radius-project/radius/pull/13404)
- Pr followup: - cover rejected parent API versions when deleting child resources by [@nicolejms](https://github.com/nicolejms) in [#13380](https://github.com/radius-project/radius/pull/13380)
- Bump versions by [@radius-dependabot-manager[bot]](https://github.com/apps/radius-dependabot-manager) in [#13401](https://github.com/radius-project/radius/pull/13401)
- Remove remaining Spec Kit agent definitions by [@Copilot](https://github.com/Copilot) in [#13400](https://github.com/radius-project/radius/pull/13400)
- Updates by [@radius-resource-types[bot]](https://github.com/apps/radius-resource-types) in [#13314](https://github.com/radius-project/radius/pull/13314)
- Bump to 1.27.3 by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13385](https://github.com/radius-project/radius/pull/13385)
- Cover direct-module object parameters and conditions by [@Copilot](https://github.com/Copilot) in [#13375](https://github.com/radius-project/radius/pull/13375)
- Synced file(s) with radius-project/.github by [@radius-files-sync[bot]](https://github.com/apps/radius-files-sync) in [#13371](https://github.com/radius-project/radius/pull/13371)
- Add authz functional test suite and rogue client helper by [@sk593](https://github.com/sk593) in [#13214](https://github.com/radius-project/radius/pull/13214)
- Use shared PR lifecycle label automation by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13356](https://github.com/radius-project/radius/pull/13356)
- Label automated resource type update PRs by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13360](https://github.com/radius-project/radius/pull/13360)
- Correct RAD_LOCATION override in CLI installation example by [@allexistence](https://github.com/allexistence) in [#13318](https://github.com/radius-project/radius/pull/13318)
- Simplify local non-cloud functional testing by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13270](https://github.com/radius-project/radius/pull/13270)
- Bump versions by [@radius-dependabot-manager[bot]](https://github.com/apps/radius-dependabot-manager) in [#13338](https://github.com/radius-project/radius/pull/13338)
- Harden release discovery and snapshot builds by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12970](https://github.com/radius-project/radius/pull/12970)
- Use github.sha for trusted pull_request_target checkouts by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13315](https://github.com/radius-project/radius/pull/13315)
- Re-enable custom terraform recipe functional test by [@nicolejms](https://github.com/nicolejms) in [#13312](https://github.com/radius-project/radius/pull/13312)
- Gate publication and automate coordination by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12948](https://github.com/radius-project/radius/pull/12948)
- Bump versions by [@radius-dependabot-manager[bot]](https://github.com/apps/radius-dependabot-manager) in [#13310](https://github.com/radius-project/radius/pull/13310)
- Clarify reviewer testing instructions in PR descriptions by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13309](https://github.com/radius-project/radius/pull/13309)
- Decouple functional-test teardown from rad app delete and upd background cleanup logging by [@lakshmimsft](https://github.com/lakshmimsft) in [#13260](https://github.com/radius-project/radius/pull/13260)
- Deploy the functional-test environment from a Bicep template by [@lakshmimsft](https://github.com/lakshmimsft) in [#13283](https://github.com/radius-project/radius/pull/13283)
- Capture Terraform provider failure diagnostics by [@nicolejms](https://github.com/nicolejms) in [#13289](https://github.com/radius-project/radius/pull/13289)
- Replace skipped Terraform PostgreSQL test with Azure provider coverage by [@nicolejms](https://github.com/nicolejms) in [#13296](https://github.com/radius-project/radius/pull/13296)
- Introduce release controller and swap triggers by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12872](https://github.com/radius-project/radius/pull/12872)
- Retry registry image pull in create-local-registry by [@nicolejms](https://github.com/nicolejms) in [#13276](https://github.com/radius-project/radius/pull/13276)
- Update RBAC feature spec for multi-resource operations by [@willtsai](https://github.com/willtsai) in [#13266](https://github.com/radius-project/radius/pull/13266)
- [stack 3/3: default k8s recipe pack] document the compiled Kubernetes recipe pack by [@nithyatsu](https://github.com/nithyatsu) in [#13250](https://github.com/radius-project/radius/pull/13250)
- Bump Go to 1.27.2 by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13263](https://github.com/radius-project/radius/pull/13263)
- Bump versions by [@radius-dependabot-manager[bot]](https://github.com/apps/radius-dependabot-manager) in [#13262](https://github.com/radius-project/radius/pull/13262)
- Add S3 and Azure backend lifecycle E2E coverage and pin the Radius.Core API version by [@lakshmimsft](https://github.com/lakshmimsft) in [#13110](https://github.com/radius-project/radius/pull/13110)
- Add ranked resource-type catalog design note by [@Reshrahim](https://github.com/Reshrahim) in [#13122](https://github.com/radius-project/radius/pull/13122)
- Add guide for adding a recipe pack for a new platform by [@nithyatsu](https://github.com/nithyatsu) in [#13182](https://github.com/radius-project/radius/pull/13182)
- Updates by [@radius-resource-types[bot]](https://github.com/apps/radius-resource-types) in [#13254](https://github.com/radius-project/radius/pull/13254)
- Cut tag releases over to GoReleaser by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12828](https://github.com/radius-project/radius/pull/12828)
- Skip managed-secret cleanup tests in the long-running workflow by [@AzureMike](https://github.com/AzureMike) in [#13253](https://github.com/radius-project/radius/pull/13253)
- Bump the javascript-deps group across 1 directory with 2 updates by [@dependabot[bot]](https://github.com/apps/dependabot) in [#13238](https://github.com/radius-project/radius/pull/13238)
- Update RBAC spec with `rad auth` command details by [@willtsai](https://github.com/willtsai) in [#13235](https://github.com/radius-project/radius/pull/13235)
- Re-enable recipe-pack tests in the long-running workflow by [@AzureMike](https://github.com/AzureMike) in [#13229](https://github.com/radius-project/radius/pull/13229)
- Add internal component authorization design note by [@sk593](https://github.com/sk593) in [#13086](https://github.com/radius-project/radius/pull/13086)
- Configure PR lifecycle labels from JSON by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13197](https://github.com/radius-project/radius/pull/13197)
- Automate release preparation and backports by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12823](https://github.com/radius-project/radius/pull/12823)
- Adopt dotted RC identifiers by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12793](https://github.com/radius-project/radius/pull/12793)
- Bump versions by [@radius-dependabot-manager[bot]](https://github.com/apps/radius-dependabot-manager) in [#13207](https://github.com/radius-project/radius/pull/13207)
- Synced file(s) with radius-project/.github by [@radius-files-sync[bot]](https://github.com/apps/radius-files-sync) in [#13192](https://github.com/radius-project/radius/pull/13192)
- Fix race and vacuous assertions in Flux functional tests by [@nicolejms](https://github.com/nicolejms) in [#13187](https://github.com/radius-project/radius/pull/13187)
- Functional spec for built-in Radius RBAC by [@willtsai](https://github.com/willtsai) in [#13089](https://github.com/radius-project/radius/pull/13089)
- Rename azure recipe pack to azure-aks and add azure-aci by [@zachcasper](https://github.com/zachcasper) in [#12662](https://github.com/radius-project/radius/pull/12662)
- Release recipe packs and publish Bicep recipes during Radius releases by [@nithyatsu](https://github.com/nithyatsu) in [#13153](https://github.com/radius-project/radius/pull/13153)
- Add functional tests for rad upgrade kubernetes by [@nicolejms](https://github.com/nicolejms) in [#13157](https://github.com/radius-project/radius/pull/13157)
- Release v0.61.1 by [@brooke-hamilton](https://github.com/brooke-hamilton) in [#13163](https://github.com/radius-project/radius/pull/13163)
- Correlate publisher runs by release identifier by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12783](https://github.com/radius-project/radius/pull/12783)
- Hand off approved review feedback to authors by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13162](https://github.com/radius-project/radius/pull/13162)
- Hand off unresolved human review threads to authors by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13148](https://github.com/radius-project/radius/pull/13148)
- Derive expected resourceNameHash instead of hardcoding it by [@kachawla](https://github.com/kachawla) in [#13117](https://github.com/radius-project/radius/pull/13117)
- Require the Conventional Commit title check by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#12763](https://github.com/radius-project/radius/pull/12763)
- Bump versions by [@radius-dependabot-manager[bot]](https://github.com/apps/radius-dependabot-manager) in [#13125](https://github.com/radius-project/radius/pull/13125)
- Release resource type namespaces before Radius by [@sk593](https://github.com/sk593) in [#13121](https://github.com/radius-project/radius/pull/13121)
- Bump supported version to v0.61.0 and add release notes by [@sk593](https://github.com/sk593) in [#13107](https://github.com/radius-project/radius/pull/13107)
- Retry post-deploy port-forward probe with backoff by [@pujitha24](https://github.com/pujitha24) in [#12972](https://github.com/radius-project/radius/pull/12972)
- Automate pull request status labels by [@DariuszPorowski](https://github.com/DariuszPorowski) in [#13106](https://github.com/radius-project/radius/pull/13106)

### Contributors

[@radius-release[bot]](https://github.com/apps/radius-release), [@AzureMike](https://github.com/AzureMike), [@DariuszPorowski](https://github.com/DariuszPorowski), [@brooke-hamilton](https://github.com/brooke-hamilton), [@kachawla](https://github.com/kachawla), [@nicolejms](https://github.com/nicolejms), [@willdavsmith](https://github.com/willdavsmith), [@Reshrahim](https://github.com/Reshrahim), [@Copilot](https://github.com/Copilot), [@radius-dependabot-manager[bot]](https://github.com/apps/radius-dependabot-manager), [@radius-files-sync[bot]](https://github.com/apps/radius-files-sync), [@vinayada1](https://github.com/vinayada1), [@dependabot[bot]](https://github.com/apps/dependabot), [@radius-resource-types[bot]](https://github.com/apps/radius-resource-types), [@sk593](https://github.com/sk593), [@lakshmimsft](https://github.com/lakshmimsft), [@itpropro](https://github.com/itpropro), [@allexistence](https://github.com/allexistence), [@willtsai](https://github.com/willtsai), [@nithyatsu](https://github.com/nithyatsu), [@pujitha24](https://github.com/pujitha24), [@zachcasper](https://github.com/zachcasper)

### New contributors

- [@radius-release[bot]](https://github.com/apps/radius-release) made their first contribution in [#13519](https://github.com/radius-project/radius/pull/13519)
- [@itpropro](https://github.com/itpropro) made their first contribution in [#13327](https://github.com/radius-project/radius/pull/13327)
- [@allexistence](https://github.com/allexistence) made their first contribution in [#13318](https://github.com/radius-project/radius/pull/13318)


## [0.60.0] - 2026-08-19

### Added

- Added preview support for fully extensible, recipe-driven resource types in the `Radius.*` namespace.
- Added direct use of standard Bicep and Terraform modules as Radius Recipes.

### Changed

- Replaced SHA-1 with SHA-256 for internal resource identifiers, Terraform recipe state keys, ETags, and change-detection tokens while preserving compatibility with existing state.

For curated highlights, upgrade guidance, contributors, and the complete change list, see the [Radius v0.60.0 release notes](docs/release-notes/v0.60.0.md).

Release history before v0.60.0 remains available on [GitHub Releases](https://github.com/radius-project/radius/releases).

[Unreleased]: https://github.com/radius-project/radius/compare/v0.62.0...HEAD
[0.62.0]: https://github.com/radius-project/radius/compare/v0.61.1...v0.62.0
[0.60.0]: https://github.com/radius-project/radius/releases/tag/v0.60.0
