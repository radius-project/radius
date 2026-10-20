# Implementation Plan: Resource Type Icons (Spec #003)

Companion to [spec.md](./spec.md). Groups the spec's functional requirements into
PR-sized slices so each slice is coherent and mergeable on its own. FR/NFR ids
reference the numbered requirements in `spec.md`.

## Slice status at a glance

| Slice | Scope | Status |
|---|---|---|
| 1 | Wire contract + inline endpoint | In progress on branch `azlinks_pr` |
| 2 | Default icon + FR-011 substitution | Complete on branch `static-graph-icons` |
| 3 | Full `--icon` grammar | Not started |
| 4 | Built-in `Applications.Core/*` icons | Not started |
| 5 | `rad app graph` + rendering integration | Not started |
| 6 | CI enforcement | Not started |

## Slice 1 — Wire contract + inline endpoint

**Delivers:** `--icon` flag, server-side hashing, `iconHash` on responses,
opt-in inline bytes, content-addressed icon endpoint, and SVG/size validation
on both the CLI and the control plane.

**FRs covered:** FR-003, FR-004, FR-005, FR-005a, FR-010, FR-012, FR-014,
FR-015, FR-018, NFR-001, NFR-002, NFR-003, NFR-004.

**FRs partially covered:**

- FR-002 — bare `--icon <path>` only, plus a multi-type guard error.
- FR-011 — `iconHash` is surfaced on responses; default substitution is Slice 2.

**Key file changes on the branch:**

- [typespec/UCP/resourceproviders.tsp](../../typespec/UCP/resourceproviders.tsp) — adds `icon` and `iconHash` to `ResourceTypeProperties` and `ResourceProviderSummaryResourceType`, and the `includeIcons` query param on the summary GET.
- [pkg/ucp/datamodel/icon_validation.go](../../pkg/ucp/datamodel/icon_validation.go) — shared `ValidateIcon` implementing FR-005 / FR-005a with a 32 KiB size cap (NFR-002).
- [pkg/ucp/api/v20231001preview/resourcetype_conversion.go](../../pkg/ucp/api/v20231001preview/resourcetype_conversion.go) — server-side validation + SHA-256 in `ConvertTo`.
- [pkg/ucp/backend/controller/resourceproviders/resourcetype_put.go](../../pkg/ucp/backend/controller/resourceproviders/resourcetype_put.go) — copies `icon`/`iconHash` onto the summary entry.
- [pkg/ucp/frontend/controller/resourceproviders/getresourceprovidersummary.go](../../pkg/ucp/frontend/controller/resourceproviders/getresourceprovidersummary.go) — honors `includeIcons` query param.
- [pkg/ucp/frontend/controller/resourceproviders/geticon.go](../../pkg/ucp/frontend/controller/resourceproviders/geticon.go) and [pkg/ucp/frontend/radius/routes.go](../../pkg/ucp/frontend/radius/routes.go) — FR-018 endpoint.
- [pkg/cli/cmd/resourcetype/create/create.go](../../pkg/cli/cmd/resourcetype/create/create.go) and [pkg/cli/manifest/registermanifest.go](../../pkg/cli/manifest/registermanifest.go) — `--icon` flag, byte read, CLI-side validation, embedding.

**Still to add inside this slice before merging:** (none required — deferred items are listed in later slices)

## Slice 2 — Default icon + FR-011 substitution

**Delivers:** every registered type has a resolvable `iconHash`, even when the
manifest arrives without an `icon` field. Same guarantee holds for the runtime
graph (`Radius.Core/applications/getGraph`), the icon endpoint (FR-018), and
the CLI-side modeled/static graph.

**FRs covered:** FR-001, FR-006, FR-011 (fully).

**Delivered on branch `static-graph-icons`:**

- [deploy/manifest/default-icon.svg](../../deploy/manifest/default-icon.svg) —
  canonical product default, hand-committed (sourced from
  `resource-types-contrib/docs/generic-resource-type.svg`).
- [deploy/manifest/icons.go](../../deploy/manifest/icons.go) — `package manifest`
  exposing `Lookup(type)`, `Default()`, `IsDefault(hash)`. Uses `go:embed`
  on the default plus the per-type SVGs synced by `make sync-resource-types`;
  no separate mirror step. Tests in
  [icons_test.go](../../deploy/manifest/icons_test.go) pin the invariants.
- [pkg/ucp/initializer/service.go](../../pkg/ucp/initializer/service.go)
  and [pkg/ucp/api/v20231001preview/resourcetype_conversion.go](../../pkg/ucp/api/v20231001preview/resourcetype_conversion.go)
  substitute the default hash at registration time. Bytes are NOT stored on
  the record — they live in the binary via the embed above.
- [pkg/corerp/frontend/controller/applications/v20250801preview/graphicons.go](../../pkg/corerp/frontend/controller/applications/v20250801preview/graphicons.go)
  and [getgraph.go](../../pkg/corerp/frontend/controller/applications/v20250801preview/getgraph.go)
  make the runtime graph always attach `iconHash` per node (falling back to
  default when a namespace 404s) and substitute default bytes into the
  `icons` map when `includeIcons: true`.
- [pkg/ucp/frontend/controller/resourceproviders/geticon.go](../../pkg/ucp/frontend/controller/resourceproviders/geticon.go)
  serves the embedded default bytes when the requested URL hash matches the
  stored `IconHash` and the record's `Icon` field is nil.
- [pkg/cli/graph/modeled.go](../../pkg/cli/graph/modeled.go) resolves
  per-node `iconHash` via the same package for the static/modeled graph;
  the response's `icons` map is populated locally without a control-plane
  call.
- Doc: [docs/architecture/application-graph.md](../../docs/architecture/application-graph.md)
  updated with the new "Default icon and FR-011 substitution" and "Static
  (modeled) graph" sections; class diagram reflects the always-set
  `iconHash`.

**Deferred to a later slice:** build-time validation that the default SVG
is valid per NFR-001 / NFR-002 (Slice 6 CI enforcement).

## Slice 3 — Full `--icon` grammar

**Delivers:** the named form and repeatable flag; strict-mode YAML decoder.

**FRs covered:** FR-002 (fully), FR-002a, FR-002b (fully), FR-002c, FR-010a.

**Changes:**

- Make `--icon` a repeatable `StringSlice`; parse each value as either `<typeName>=<path>` or bare `<path>`.
- Enforce per-type resolution rules; error on duplicate targeting, ambiguous args, non-matching typeName.
- Switch the manifest YAML decoder to known-fields mode so `icon:` under a type is a hard error.

## Slice 4 — Built-in `Applications.Core/*` icons

**Delivers:** every built-in type ships with its own distinct custom icon.

**FRs covered:** FR-007, FR-008, FR-009. Success criterion: SC-003.

**Changes:**

- Add SVG assets for `environments`, `applications`, `containers`, `gateways`, `secretStores`, `extenders`.
- Regenerate `deploy/manifest/built-in-providers/{dev,self-hosted}/` through the icon-embedding flow.
- Wire a build-time mapping (`Makefile` or generator) so the icon-to-type association is explicit, not filename-inferred.

## Slice 5 — `rad app graph` + rendering integration

**Delivers:** graph responses carry `iconHash` per node; the dashboard can
consume them.

**FRs covered:** FR-013, FR-016, FR-017, FR-019, NFR-005 (renderer-side).

**Changes in this repo:**

- Extend the `getGraph` response to include `iconHash` per node.
- Add an `includeIcons` query param that returns an `icons` map deduped by hash (FR-013).
- Add `--include-icons` to `rad app graph`.

**Changes in the dashboard repo (separate PR):** consume `iconHash` and either
use inline bytes or fetch them via FR-018.

## Slice 6 — CI enforcement

**Delivers:** malformed / oversized / non-SVG icons are rejected in CI.

**FRs covered:** FR-020. Success criterion: SC-005.

**Changes:**

- CI job that validates every referenced icon file plus the default icon against NFR-001 and NFR-002.

## What Slice 1 delivers today (functional summary)

- `rad resource-type create <typeName> --from-file <yaml> --icon <path.svg>` reads the SVG and sends its verbatim UTF-8 bytes as `ResourceTypeProperties.icon`.
- Multi-type YAML combined with a bare `--icon` fails with an actionable CLI error.
- The server computes `iconHash` (SHA-256 of the bytes) during conversion and stores both `icon` and `iconHash`.
- The provider summary API returns `iconHash` for every type by default; add `?includeIcons=true` to also inline the bytes.
- `GET /planes/radius/{plane}/providers/System.Resources/resourceProviders/{namespace}/resourceTypes/{type}/icons/{hash}` returns the raw SVG bytes with `Content-Type: image/svg+xml; charset=utf-8` and a long-lived `Cache-Control`.

Explicitly **not** on the branch yet: default icon, SVG/size validation on
either side, named `--icon <typeName>=<path>` form, YAML strict-mode for the
`icon:` key, built-in `Applications.Core/*` icons, graph endpoint changes,
`rad app graph --include-icons`, CI validation, and the FR-018 `ETag` header.

## Manual verification — Slice 1

Copy the block below into a test ticket.

````markdown
## Manual verification — Slice 1 (icons wire contract + inline endpoint)

### Preconditions

- Local debug stack running: `make debug-start` (UCP listens on `localhost:9000`).
- `./drad` built at the repo root (produced by `make debug-start`).
- `curl`, `jq`, `python3`, and `shasum` (or `sha256sum`) available.

### 1. Prepare fixtures

```bash
cat > /tmp/rt.yaml <<'YAML'
namespace: MyCompany.Resources
types:
  testResources:
    description: This is a test resource type.
    apiVersions:
      "2025-01-01-preview":
        schema: {}
YAML

cat > /tmp/icon.svg <<'SVG'
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><circle cx="50" cy="50" r="40" fill="red"/></svg>
SVG

cat > /tmp/rt-multi.yaml <<'YAML'
namespace: MyCompany.Multi
types:
  a:
    apiVersions: { "2025-01-01-preview": { schema: {} } }
  b:
    apiVersions: { "2025-01-01-preview": { schema: {} } }
YAML
```

### 2. Register a type with an icon (FR-004, FR-010, FR-012)

```bash
./drad resource-type create testResources -f /tmp/rt.yaml --icon /tmp/icon.svg
```

**Expected:** `MyCompany.Resources/testResources created`.

### 3. Verify the hash is deterministic and server-computed (FR-010)

```bash
EXPECTED_HASH=$(shasum -a 256 /tmp/icon.svg | awk '{print $1}')
ACTUAL_HASH=$(curl -s "http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local/providers/MyCompany.Resources" \
  | jq -r '.properties.resourceTypes.testResources.iconHash')
echo "expected=$EXPECTED_HASH"
echo "actual  =$ACTUAL_HASH"
[ "$EXPECTED_HASH" = "$ACTUAL_HASH" ] && echo "match" || echo "mismatch"
```

**Expected:** `match` — the server's `iconHash` equals SHA-256 of the SVG file bytes.

### 4. Default response omits bytes; opt-in returns them (FR-015)

```bash
# Default: no bytes, only hash
curl -s "http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local/providers/MyCompany.Resources" \
  | jq '.properties.resourceTypes.testResources | {has_icon: (.icon != null), iconHash}'

# Opt-in: bytes inlined
curl -s "http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local/providers/MyCompany.Resources?includeIcons=true" \
  | jq '.properties.resourceTypes.testResources | {icon,iconHash}'
```

**Expected:**

- Default call → `{"has_icon": false, "iconHash": "<hash>"}`.
- `includeIcons=true` call → both `icon` (the SVG string) and `iconHash` populated.

### 5. Fetch bytes via the content-addressed endpoint (FR-018)

```bash
HASH=$(curl -s "http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local/providers/MyCompany.Resources" \
  | jq -r '.properties.resourceTypes.testResources.iconHash')

curl -i "http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local/providers/System.Resources/resourceproviders/MyCompany.Resources/resourcetypes/testResources/icons/$HASH"
```

**Expected:**

- `HTTP/1.1 200 OK`
- `Content-Type: image/svg+xml; charset=utf-8`
- `Cache-Control: public, max-age=31536000, immutable`
- Body equals the exact contents of `/tmp/icon.svg`.

### 6. Wrong hash returns 404 (FR-018 stale-hash case)

```bash
curl -i "http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local/providers/System.Resources/resourceproviders/MyCompany.Resources/resourcetypes/testResources/icons/deadbeefdeadbeef"
```

**Expected:** `HTTP/1.1 404 Not Found` with a JSON error body.

### 7. Multi-type YAML with bare `--icon` is rejected (FR-002b, partial)

```bash
./drad resource-type create -f /tmp/rt-multi.yaml --icon /tmp/icon.svg
```

**Expected:** non-zero exit, error message telling the author to specify a
single type name (currently `rad resource-type create <typeName> --from-file <file> --icon <path>`).

### 8. CLI rejects non-SVG icon (FR-005, NFR-001)

```bash
cat > /tmp/bad.svg <<'HTML'
<html><body>not an svg</body></html>
HTML

./drad resource-type create testResources -f /tmp/rt.yaml --icon /tmp/bad.svg
```

**Expected:** non-zero exit, error message containing `invalid icon file` and
`root element is <html>, expected <svg>`. Nothing is sent to the control plane.

### 9. CLI rejects oversized icon (FR-005, NFR-002)

```bash
python3 -c "print('<svg xmlns=\"http://www.w3.org/2000/svg\">' + 'x' * 33000 + '</svg>')" > /tmp/big.svg

./drad resource-type create testResources -f /tmp/rt.yaml --icon /tmp/big.svg
```

**Expected:** non-zero exit, error message containing `exceeds the 32768 byte limit`.

### 10. Server rejects malicious SVG (FR-005a)

Even if the CLI check is bypassed (for example a caller hitting the wire API
directly), the server rejects dangerous SVG bytes.

```bash
cat > /tmp/scripty.svg <<'SVG'
<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>
SVG

./drad resource-type create testResources -f /tmp/rt.yaml --icon /tmp/scripty.svg
```

**Expected:** non-zero exit. The failure is raised by the CLI validator here;
in a bypass scenario the control plane returns `HTTP 400` with an
`invalid icon` message.

### 11. Type without an icon has no `iconHash` today (Slice 2 will fix)

```bash
./drad resource-type create testResources -f /tmp/rt.yaml
curl -s "http://localhost:9000/apis/api.ucp.dev/v1alpha3/planes/radius/local/providers/MyCompany.Resources" \
  | jq '.properties.resourceTypes.testResources | {icon, iconHash}'
```

**Expected (Slice 1):** `iconHash` is `null` / absent.
**Expected once Slice 2 lands:** `iconHash` equals the default icon's hash.

### Cleanup

```bash
rm -f /tmp/rt.yaml /tmp/rt-multi.yaml /tmp/icon.svg /tmp/bad.svg /tmp/big.svg /tmp/scripty.svg
```
````
