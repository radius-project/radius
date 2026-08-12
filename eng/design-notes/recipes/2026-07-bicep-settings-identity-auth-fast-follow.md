# Topic: BicepSettings Identity Authentication Fast Follow

- **Author**: Sylvain Niles
- **Status**: Draft for review

## Topic Summary

`Radius.Core/bicepSettings` lets a platform engineer configure how Radius authenticates to private Bicep registries when pulling recipe templates. Authentication is keyed by registry hostname, and each entry declares an `authenticationMethod`: `BasicAuth` (username/password from a secret), `AzureWI` (Azure Workload Identity), or `AwsIrsa` (AWS IAM Roles for Service Accounts). The public schema and controller validation accept all three methods today, but only `BasicAuth` is actually honored when a recipe runs — `AzureWI` and `AwsIrsa` are accepted, persisted, and then silently ignored at execution time.

This document specifies the fast-follow work to make `AzureWI` and `AwsIrsa` effective end-to-end during recipe execution, while keeping existing `BasicAuth` flows unchanged. It is intentionally scoped to completing the runtime behind the *existing* API contract. It does not introduce new REST resource types, new fields, or a new auth model.

## Context and Problem

### How bicepSettings reaches recipe execution

`Radius.Core/bicepSettings` is a standalone, resource-group–scoped resource that an environment references. It is not consumed directly by the recipe engine. Instead, its configuration is translated into the internal shape the recipe engine understands. That translation is the **Radius.Core environment bridge**, and it is the crux of this design:

1. **Persisted resource (public API shape).** `bicepSettings.registryAuthentications` is a map of `host -> { authenticationMethod, basicAuthSecretId, azureWi*, awsIam* }`. This is what the user writes and what validation accepts.
2. **Environment bridge (translation point).** When a recipe deploys, the environment config loader ([`pkg/recipes/configloader/environment.go`](../../../pkg/recipes/configloader/environment.go)) reads the referenced `bicepSettings` and maps its host-keyed entries into the internal, execution-time `recipes.Configuration` — specifically the legacy `RecipeConfig.Bicep.Authentication` map of `host -> RegistrySecretConfig{ Secret, AuthenticationMethod }`. This bridge exists because the persisted resource shape and the runtime configuration shape are deliberately decoupled; the loader is the single place they are reconciled.
3. **Bicep driver (consumer).** The driver ([`pkg/recipes/driver/bicep/bicep.go`](../../../pkg/recipes/driver/bicep/bicep.go)) looks up the entry by the registry host parsed from the template path, then selects a registry auth client (`authclient.GetNewRegistryAuthClient`, via [`pkg/rp/util/registry.go`](../../../pkg/rp/util/registry.go)) using the bridged method and credential data.

The checked-in architecture document ([docs/architecture/terraform-bicep-settings.md](../../../docs/architecture/terraform-bicep-settings.md), §What is excluded) already calls out this runtime gap as a follow-up.

### The gap this fast-follow fills

The bridge is where identity methods are lost, for two compounding reasons:

- **The bridge drops non-secret entries.** The loader iterates `registryAuthentications` and skips any entry whose `basicAuthSecretId` is empty (`if auth.BasicAuthSecretId == "" { continue }`). `AzureWI` and `AwsIrsa` carry no secret ID — they authenticate with inline, non-secret identity metadata (`azureWiClientId`/`azureWiTenantId`, or `awsIamRoleArn`) — so those entries never survive the bridge and never reach the driver.
- **The internal descriptor has nowhere to put identity data.** Even if forwarded, the runtime `RegistrySecretConfig` only carries `{ Secret, AuthenticationMethod }`. It has no fields for client/tenant IDs or a role ARN, so the identity inputs would be discarded even if the entry were not skipped.

The result is a gap between accepted configuration and effective execution behavior:

- Users can create and persist `AzureWI` and `AwsIrsa` configurations successfully (the API says yes).
- Recipe execution silently ignores those entries, because the bridge only forwards `basicAuthSecretId` entries and the runtime type cannot represent identity inputs.
- Correctness therefore depends on `BasicAuth` even when a different, valid method is configured — with no error to signal the mismatch.

This fast-follow closes that gap by extending the internal runtime descriptor with identity fields, making the bridge forward every method (not just secret-backed ones), and making the driver materialize the correct auth client per method — so the configuration a user is already allowed to persist becomes the configuration that actually runs.

## Goals

1. Make `AzureWI` and `AwsIrsa` in `Radius.Core/bicepSettings` effective at recipe execution time.
2. Keep `BasicAuth` behavior unchanged and compatible with both `Radius.Security/secrets` and `Applications.Core/secretStores`.
3. Eliminate silent skip behavior for valid non-BasicAuth entries.
4. Preserve the existing public REST schema for `bicepSettings` (no breaking API changes).
5. Provide deterministic, test-covered behavior for all three methods.

## Non-goals

1. No redesign of the `bicepSettings` schema.
2. No new auth method additions.
3. No migration tooling for existing resources.
4. No changes to Terraform settings behavior.

## Desired End State

For each `registryAuthentications[host]` entry:

- `BasicAuth`: runtime uses secret-backed username/password credentials.
- `AzureWI`: runtime uses `azureWiClientId` + `azureWiTenantId` from the settings entry (no secret dependency).
- `AwsIrsa`: runtime uses `awsIamRoleArn` from the settings entry (no secret dependency).

If the configured method cannot be materialized at runtime, execution fails with a clear error instead of silently skipping the host.

## Detailed Design

### 1. Bridge all methods from environment loader

Update the `Radius.Core` environment bridge logic so it forwards all configured auth entries, not only those with `basicAuthSecretId`.

Current issue:

- Bridge filters out entries with empty `basicAuthSecretId`.
- This implicitly drops `AzureWI`/`AwsIrsa`.

Change:

- Build a method-specific runtime auth descriptor per host.
- Keep host as the lookup key.
- Preserve `AuthenticationMethod` as explicit runtime selector.

### 2. Extend runtime auth descriptor shape

The shared runtime type currently carries only secret reference + method. Add explicit optional fields for identity-based methods so runtime has method-specific inputs without relying on secrets.

Proposed `RegistrySecretConfig` additions:

- `AzureWiClientId string`
- `AzureWiTenantId string`
- `AwsIamRoleArn string`

Behavior:

- `BasicAuth` uses `Secret`.
- `AzureWI` uses inline `AzureWiClientId` and `AzureWiTenantId`.
- `AwsIrsa` uses inline `AwsIamRoleArn`.

### 3. Secret prefetch behavior by method

`FindSecretIDs` in the Bicep driver should request secrets only when needed.

Change:

- For `BasicAuth`, include `Secret` in prefetch map.
- For `AzureWI` and `AwsIrsa`, do not request secret data.
- Ignore empty secret IDs safely.

This avoids accidental secret loader calls with empty IDs and aligns data dependencies with method semantics.

### 4. Build auth payload for authclient deterministically

Current authclient factory consumes `recipes.SecretData` with a `Type` and key-value map. Keep that contract, but produce data per method:

- `BasicAuth`: from fetched secret values (`username`, `password`) and type mapping.
- `AzureWI`: synthesize `recipes.SecretData` with type `azureWorkloadIdentity` and data keys `clientId`, `tenantId` from bridged fields.
- `AwsIrsa`: synthesize `recipes.SecretData` with type `awsIRSA` and data key `roleARN` from bridged fields.

The runtime path should not depend on the secret resource kind for identity methods.

### 5. Strengthen runtime error semantics

Validation already enforces conditional required fields at controller level. Runtime should still fail clearly if malformed data reaches execution (defense in depth).

Examples:

- `AzureWI` missing `clientId` or `tenantId` at execution path => explicit error naming host + missing fields.
- `AwsIrsa` missing role ARN => explicit error naming host.
- Unknown auth method => explicit error listing supported methods.

### 6. Preserve backward compatibility

- Existing `BasicAuth` resources continue to work as-is.
- Existing secret types and key conventions remain unchanged.
- No API version change required for this fast-follow because schema already includes needed fields.

## Component-level Changes

### Core RP / Datamodel

- Update runtime datamodel type for Bicep registry auth descriptor to include identity fields.
- Ensure conversions used by runtime preserve method-specific values.

### Recipe Config Loader

- Update environment bridge logic to map all methods and fields.
- Remove silent skip behavior for non-BasicAuth entries.

### Bicep Driver

- Update `FindSecretIDs` to be method-aware.
- Update registry auth materialization logic so each method builds correct auth payload for authclient.

### Shared Util / Auth Mapping

- Refactor registry secret resolution helper into method-aware resolver (secret-backed vs synthesized).
- Keep existing authclient factory contract.

## Testing Plan

### Unit Tests

#### Bridge tests

- `BasicAuth` maps secret + method.
- `AzureWI` maps client/tenant + method and is not dropped.
- `AwsIrsa` maps role ARN + method and is not dropped.

#### Driver secret ID discovery tests

- `BasicAuth` returns secret ID.
- `AzureWI`/`AwsIrsa` return no secret IDs.

#### Auth payload construction tests

- BasicAuth pulls username/password from fetched secrets.
- AzureWI synthesizes `clientId`/`tenantId`.
- AwsIrsa synthesizes `roleARN`.

#### Error-path tests

- Missing method-required fields generate explicit runtime errors.
- Unknown method fails fast.

### Functional Tests

Add or extend noncloud functional tests for `Radius.Core/bicepSettings`:

1. OCI pull with `BasicAuth` still succeeds.
2. OCI pull with `AzureWI` path reaches identity auth client selection (success test where feasible; otherwise deterministic failure proving method was attempted).
3. OCI pull with `AwsIrsa` path reaches identity auth client selection (same approach).

If full external identity integration cannot be validated in CI, include deterministic integration tests at the boundary where auth client selection occurs and document external validation steps.

## Observability and Diagnostics

Add structured logs around auth method selection per host:

- host
- selected method
- whether secret lookup was required

Do not log secret values.

## Security Considerations

- Identity fields (`clientId`, `tenantId`, `roleARN`) are non-secret metadata and can remain in settings.
- Secret-backed credentials remain in secret stores and are only read for `BasicAuth`.
- No new plaintext secret persistence paths are introduced.

## Rollout and Risk

Risk is moderate and bounded to Bicep private registry authentication path.

Mitigations:

- Keep method-specific logic isolated and unit tested.
- Preserve existing BasicAuth flow untouched where possible.
- Add guardrail runtime errors instead of silent fallback/skip behavior.

## Acceptance Criteria

1. A `bicepSettings` entry with `authenticationMethod: AzureWI` is used during recipe execution and is not silently ignored.
2. A `bicepSettings` entry with `authenticationMethod: AwsIrsa` is used during recipe execution and is not silently ignored.
3. `BasicAuth` behavior remains backward compatible.
4. Method-specific missing field cases fail with explicit, host-scoped error messages.
5. Unit + functional coverage is added for all three methods.

## Implementation Plan (Suggested PR breakdown)

1. PR 1: Runtime/datamodel/bridge updates + unit tests.
2. PR 2: Driver method-aware auth materialization + unit tests.
3. PR 3: Functional tests + docs update removing limitation note.

## Draft Issue Text (copy/paste)

Title: Complete `Radius.Core/bicepSettings` runtime support for `AzureWI` and `AwsIrsa`

Summary:
`Radius.Core/bicepSettings` schema and validation support `BasicAuth`, `AzureWI`, and `AwsIrsa`, but runtime execution currently only wires `BasicAuth`. Complete end-to-end execution support for `AzureWI` and `AwsIrsa` in the Bicep recipe path.

Scope:

- Method-aware environment bridge for `bicepSettings.registryAuthentications`
- Runtime auth descriptor updates for identity fields
- Bicep driver updates for method-aware secret prefetch and auth payload creation
- Unit + functional tests
- Remove "AzureWI/AwsIrsa are no-op" limitation from architecture docs once complete

Out of scope:

- Schema redesign
- New auth methods
- Terraform behavior changes

Done when:

- All three methods are effective at execution time
- No silent skip for non-BasicAuth entries
- Tests and diagnostics cover method selection/error paths
