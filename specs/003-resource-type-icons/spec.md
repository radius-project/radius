# Feature Specification: Resource Type Icons

**Feature Branch**: `21027-resource-type-icons`  
**Spec Directory**: `specs/003-resource-type-icons`  
**Created**: 2026-05-06  
**Status**: Draft  
**Input**: Add an "icon" capability to Radius resource types so that every resource type — contributor-authored types defined in `resource-types-contrib`, manifest-driven types shipped under `deploy/manifest/built-in-providers/`, and the first-class core types in the `Applications.Core/*` namespace — can ship a custom visual identity that flows from authoring through the control plane to the application graph rendered by the dashboard and the rad CLI.

## Purpose

Icons exist to give every resource type a recognizable visual identity for use in graph layouts that render Radius applications — primarily the dashboard's application-graph view and any other surface that draws nodes for deployed resources. A scannable, per-type icon turns a graph of opaque boxes into one where users can identify resource types at a glance, distinguish similar topologies, and orient themselves quickly in unfamiliar applications. This feature delivers the end-to-end mechanism (authoring, build, API, render) that makes that visual identity possible.

## Assumptions

This section captures every premise the rest of the spec relies on. They are *not* requirements — if any of them turn out to be false during planning, the spec must be revisited.

### Conventions chosen as reasonable defaults

- **A-004**: The currently in-use preview API versions are the targets for the additive `icon` field. No new API version is introduced.

### Radius platform assumptions

- **A-009**: Radius resource types come from two authoring sources: (i) contributor-authored types in `resource-types-contrib`, published as Bicep extensions, and (ii) built-in types declared in the manifests under `deploy/manifest/built-in-providers/{dev,self-hosted}/`. The `Applications.Core/*` namespace (`environments`, `applications`, `containers`, `gateways`, `secretStores`, `extenders`) is part of the second source — it ships via the same manifest publishing flow as the other built-in providers (`Radius.Compute`, `Radius.Data`, `Applications.Dapr`, etc.), not as a separately implemented control-plane-only namespace.
- **A-010**: Both authoring sources pass through a build step before reaching the control plane: contrib types go through `make build-resource-type` → `rad bicep publish-extension`, and built-in types go through the manifest-generation step that writes `deploy/manifest/built-in-providers/{dev,self-hosted}/`. An icon-resolution stage can be added to each of these existing steps; no new tooling or pipeline is required. (The requirement that the stage exists is FR-004 and FR-008.)
- **A-011**: Radius's control plane (UCP) hosts the resource-type registry. Resource types reach the control plane either via (a) `rad resource-type create` against a published manifest, or (b) the built-in provider manifests under `deploy/manifest/built-in-providers/{dev,self-hosted}/` loaded at control-plane startup. (The requirement that both paths expose icons through the same API contract is FR-011.)
- **A-012**: The application graph is computed by the Core resource provider via a single `getGraph` operation. The CLI's `rad app graph` and the dashboard both consume that one endpoint, so adding the icon to its response is sufficient to cover both rendering surfaces.
- **A-013**: Authentication and authorization for the resource-type and application-graph endpoints are unchanged. The icon rides inside existing responses and inherits their access controls.
- **A-014**: Some resource types reach the control plane through paths that don't run the icon-resolution build step described in A-010 — for example, types registered manually via `rad resource-type create` against a hand-edited manifest, and types from any future registration path. These types may arrive at the control plane with no icon attached. (The runtime fallback behavior that handles this case is FR-027.)
- **A-015**: TypeSpec is the source of truth for the affected API models; Go datamodel structures and wire types are generated from TypeSpec. Schema changes are made in TypeSpec first, with codegen propagating them downward.
- **A-016**: The Radius product build can vendor a file from the contributing repository into the Radius source tree at build time (existing cross-repo tooling, a documented copy step, or equivalent). The exact mechanism is a planning concern.

### Delivery and scale assumptions

- **A-021**: Icons are **static and global** per resource type — there are no per-environment, per-tenant, per-instance, per-theme, or per-locale variants in v1.
- **A-022**: A typical application graph contains tens of nodes, but graphs with hundreds of same-type nodes are realistic. With a 32 KiB cap **and** content-hash dedup within a response (FR-013, FR-024), graph response size scales with the number of *distinct* icons (typically a handful), not with the node count. No pagination or icon-stripping query parameter is required to keep responses bounded.
- **A-023**: The dashboard is the only first-party rendering surface for icons in v1. Other Radius surfaces — in particular `rad app graph` text output and `-o json` output — do not render icons and do not need icon bytes by default. (The requirement that CLI consumers and scripting pipelines can omit icon bytes from responses is FR-025.)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Default icon for every resource type (Priority: P1)

As a developer viewing an application graph, I want every node to display a meaningful icon — even when the resource type's author has not provided one — so that the graph is visually scannable from day one without requiring per-type artwork.

**Why this priority**: This is the floor of the feature. Without a guaranteed default, the icon contract is not honored end-to-end and every downstream consumer (CLI graph JSON, dashboard) must invent its own fallback. Shipping the mechanism plus a single default icon is the minimum viable slice that delivers user value across all four repositories.

**Independent Test**: Register a resource type that declares no icon, deploy an application that uses it, and confirm that (a) the resource-type definition retrieved from the control plane carries the default icon's hash, (b) the application-graph response carries the default icon's hash on that node and resolves to the default SVG bytes (inline when `includeIcons=true`, via the icon endpoint otherwise), and (c) the dashboard renders the default icon on the corresponding graph node.

**Acceptance Scenarios**:

1. **Given** a resource type definition with no sibling icon file in the contributing repository, **When** it is published and registered with the control plane, **Then** every API that returns that resource type definition exposes the repository-wide default icon's hash, and the bytes addressed by that hash are the default SVG.
2. **Given** an application that uses a resource type with no declared icon, **When** the application graph is requested with `includeIcons=true`, **Then** the response's `icons` map contains the default icon's bytes keyed by hash, and every such node's `iconRef` points at that hash.
3. **Given** the dashboard is showing the application graph, **When** a node's resource type has no declared icon, **Then** the dashboard renders the default icon at the node.

---

### User Story 2 - Authoring a custom icon alongside a resource type (Priority: P1)

As a platform engineer authoring a resource type in the contributing repository, I want to drop a normal SVG file next to my resource type definition and have it become the type's icon, so that I get a native editing experience (open in any SVG editor, normal diffs in pull requests) without hand-editing YAML or base64-encoding anything.

**Why this priority**: Authors will not adopt an icon contract that requires hand-editing embedded XML in YAML. A clean source layout is essential to drive per-type icon adoption after the mechanism ships. This story is independently testable end-to-end and is what unlocks per-type icons populating in a follow-up.

**Independent Test**: Add a sibling `icon.svg` to an existing resource type folder, run the existing build/publish flow, and confirm the published artifact carries that icon and downstream consumers see it.

**Acceptance Scenarios**:

1. **Given** a resource type folder that contains both the type definition file and a sibling `icon.svg`, **When** the contributor runs the existing build step that publishes the resource type, **Then** the published artifact contains the SVG content embedded in the resource type definition without any manual edit to the definition file.
2. **Given** the same resource type registered with the control plane, **When** any API that returns that resource type's definition is called, **Then** the response carries the contributor-supplied icon (not the default).
3. **Given** an application uses that resource type, **When** the application graph is requested and rendered in the dashboard, **Then** the contributor-supplied icon appears on the corresponding node.

---

### User Story 3 - Built-in resource types ship with icons (Priority: P2)

As a Radius user installing the product out of the box, I want every resource type that ships with Radius by default — including the `Applications.Core/*` types like `environments`, `applications`, `containers`, and `gateways` that drive the first-run experience — to already have a custom icon attached, so that the very first application graph I render is visually meaningful without any contributor work on my part. The first thing a new user sees — their environment, application, container, and gateway nodes — must have distinct, recognizable icons, not the generic default.

**Why this priority**: Users encounter the built-in providers and the core types before they encounter contributed ones. The types that drive the user's first-run experience must carry meaningful, non-default icons end-to-end through the install pipeline so the contract is observable on a fresh install. Iconifying these is little additional work because the same manifest publishing flow already handles them.

**Independent Test**: Install Radius using the standard build, deploy a sample application that uses built-in resource types (including at minimum an environment, application, container, and gateway), request the application graph, and confirm every node carries a custom icon — not the repository-wide default — both via the API and in the dashboard.

**Acceptance Scenarios**:

1. **Given** a fresh install of Radius produced by the standard build pipeline, **When** the built-in resource type manifests are loaded into the control plane, **Then** every built-in resource type — including every `Applications.Core/*` type — has its own custom icon associated with it (its definition exposes a non-empty `iconHash` and that hash resolves to non-default SVG bytes via FR-026).
2. **Given** the same install, **When** a sample application is deployed and its graph requested with `includeIcons=true`, **Then** every node carries a non-null `iconRef` whose bytes are present in the response's `icons` map, and no built-in node falls back to the default-icon hash.
3. **Given** an application graph that contains nodes of type `Applications.Core/environments`, `applications`, `containers`, and `gateways`, **When** the graph is rendered in the dashboard, **Then** each of those nodes displays its **own custom icon** (visually distinct from the repository-wide default and from each other).

---

### User Story 4 - Contributor guidance and CI safety net (Priority: P2)

As a maintainer of the contributing repository, I want clear documentation and automated checks so that contributors who add an icon do so in a consistent, valid way, and so that pull requests cannot regress the icon contract.

**Why this priority**: Without documentation and CI enforcement, the icon contract degrades over time (oversized files, malformed SVG, missing default icon). This story protects the long-term health of the feature and is independently demonstrable via a pull request that intentionally violates the contract being rejected by CI.

**Independent Test**: Open a pull request that adds an oversized or malformed icon file (and a separate one that removes the repository-wide default) and confirm CI rejects each one with a clear error. Confirm the contributor-facing documentation describes the icon contract.

**Acceptance Scenarios**:

1. **Given** a pull request that adds a sibling icon file that is not valid SVG, **When** CI runs, **Then** the run fails with an actionable error.
2. **Given** a pull request that adds a sibling icon file exceeding the documented size cap, **When** CI runs, **Then** the run fails with an actionable error.
3. **Given** a pull request that removes or invalidates the repository-wide default icon, **When** CI runs, **Then** the run fails with an actionable error.
4. **Given** a new contributor reads the contributing guide, **When** they look for "how do I add an icon", **Then** they find the file location, format requirement, size cap, and the fact that no manual edit to the resource type definition is required.

---

### Edge Cases

- A resource type folder contains an icon file in an unsupported format (PNG, JPEG, GIF). The build step must fail with a clear error directing the contributor to provide SVG.
- A sibling icon file is present but is empty or not parseable as XML. The build step must fail before publishing.
- A sibling icon file exceeds the published size cap. The build step must fail before publishing.
- A resource type definition file already contains an embedded icon value (e.g., from a hand-edit or a previous build) and a sibling icon file is also present. The sibling file is the source of truth and the build step overwrites the embedded value (deterministic behavior, no merge).
- The repository-wide default icon is missing or invalid at build time. The Radius build that vendors the default into its binaries must fail loudly rather than silently shipping a binary with no fallback.
- A consumer of the application-graph response does not understand the new `iconRef`/`icons` fields. The response must remain valid for older consumers (additive fields).
- A consumer of the dashboard receives an empty or unresolvable icon reference for a node (defense in depth despite the API contract). The dashboard renders a built-in fallback rather than a broken image.
- A graph contains hundreds of nodes of the same resource type. The response carries one entry in `icons` for that type's hash and N `iconRef` strings on the nodes — not N copies of the SVG (FR-024).
- A request to the icon endpoint (FR-026) supplies a `{hash}` that doesn't match the stored bytes for that type (e.g. a stale dashboard cache after the type was re-published with a new icon). The endpoint returns 404; the dashboard falls back to FR-016.

## Requirements *(mandatory)*

### Functional Requirements

#### Source layout and authoring (contributing repository)

- **FR-001**: The contributing repository MUST host a single canonical default icon at the path `icons/default.svg` (relative to the repository root). The Radius product build, the per-type build step, and the contributing-repo CI all resolve the default icon from this exact location.
- **FR-002**: A resource type MAY declare a custom icon by placing a sibling file named exactly `icon.svg` (lowercase) next to its definition file. No other filename is recognised by the build step; alternative names MUST be ignored (or rejected by CI — see FR-018).
- **FR-003**: Contributors MUST NOT be required to edit the resource type definition file by hand to attach an icon. Authoring an icon means dropping or editing the sibling SVG file.

#### Build and packaging

- **FR-004**: The existing build step that turns a contributed resource type definition into a published artifact MUST also resolve the type's icon (sibling file when present, repository-wide default otherwise) and write it back into the published definition as a data element on the type, so that the published artifact is self-contained.
- **FR-005**: The build step MUST fail with an actionable error if the resolved icon is missing, not SVG, malformed, or exceeds the size cap (see NFR-002).
- **FR-006**: The Radius product build MUST vendor the contributing repository's default icon into the Radius source tree and embed it into the Radius CLI and control-plane binaries, so that the control plane can always answer with an icon even when a registered type has none.
- **FR-007**: The Radius product build MUST fail if the vendored default icon is missing or invalid, rather than producing a binary with no fallback.
- **FR-008**: The built-in provider manifests that ship with Radius by default MUST be regenerated through the same build flow so that every built-in resource type's published manifest carries its icon embedded. This includes every `Applications.Core/*` type — they participate in the manifest publishing flow alongside the other built-in providers and MUST ship with their own custom icons rather than falling back to the default.
- **FR-021**: Every built-in resource type in the `Applications.Core/*` namespace (at minimum: `environments`, `applications`, `containers`, `gateways`, `secretStores`, `extenders`) MUST ship with its own custom SVG icon embedded into the built-in provider manifest at build time and surfaced through the same icon contract as contributor-authored types. These types MUST NOT fall back to the repository-wide default icon — they are the user's first-run experience and MUST be visually distinct from each other and from the default.
- **FR-022**: Authoring or updating the icon for a built-in type (including `Applications.Core/*` types) MUST follow a sibling-file convention analogous to FR-002: contributors drop or edit a single SVG file at a well-known location colocated with the type's manifest source in the Radius repository, with no hand-editing of the manifest YAML required. The exact directory layout for built-in icons (e.g. `deploy/manifest/built-in-providers/icons/<namespace>/<typeName>.svg`) is a planning concern; the requirement is that a contributor never has to base64-encode or hand-paste SVG content into a manifest file.

#### Schema and API surface

- **FR-009**: The resource type definition schema MUST gain an optional `icon` field whose value is the SVG content as text.
- **FR-010**: Existing resource type definition files that omit the `icon` field MUST continue to validate, register, and deploy without modification.
- **FR-011**: Every API surface that returns a resource type definition MUST carry the type's resolved icon information — at minimum the icon's content hash (`iconHash`) so consumers can fetch or correlate the bytes — wherever the type's description is exposed. This includes the resource-type CRUD/list/show surfaces and the resource-provider summary surface. When the request opts in (FR-025), the response also carries the SVG bytes (deduped per FR-024).
- **FR-012**: API consumers that opt in to icon bytes MUST be able to retrieve them in a single call without a follow-up request. API consumers that don't opt in MUST be able to resolve bytes for any `iconHash` via the dedicated icon endpoint (FR-026) without re-fetching the metadata response.
- **FR-013**: The application-graph response MUST identify each node's icon by reference (`iconRef` carrying the icon's content hash). When the request opts in to icon bytes (FR-025, default `false`), the response also carries a top-level `icons` map keyed by content hash that supplies the SVG bytes for every distinct hash referenced by any node. Either way, every node MUST have a non-null `iconRef` resolvable to a non-empty SVG (custom where registered, default-icon hash otherwise).
- **FR-014**: Adding the `icon`, `iconHash`, `iconRef`, and `icons` fields MUST be backward compatible with the in-use preview API versions; existing clients that do not understand any of these fields MUST continue to function.
- **FR-020**: When SVG bytes are delivered — whether in the published resource type definition's `icon` field, in an `icons` map on a response, or as the body of the icon endpoint (FR-026) — they MUST be embedded verbatim as text. Base64 encoding, external URL references, file paths, or any other indirection MUST NOT be used in the bytes themselves. (Hash-based references via `iconRef`/`iconHash` are not indirection in this sense — they are the canonical addressing model; see FR-013 and FR-026.)
- **FR-024**: A single API response MUST NOT contain duplicate copies of the same icon's bytes. When a response carries an `icons` map alongside per-node or per-type `iconRef`/`iconHash` fields, the map MUST contain at most one entry per distinct content hash, regardless of how many references point at it.
- **FR-025**: API surfaces that can return icon bytes MUST accept an `includeIcons` query parameter. When `true`, the response body carries the bytes (per FR-011 / FR-013, deduped per FR-024). When `false` or absent, the response carries only references (`iconHash` / `iconRef`) and no bytes. **The default is `false`** so that scripting consumers (notably `rad app graph -o json`) and any other automated client never pay for bytes they cannot render. The dashboard, which actually renders SVGs, opts in explicitly.
- **FR-026**: The control plane MUST expose a dedicated icon endpoint, `GET /planes/radius/{plane}/providers/{namespace}/resourceTypes/{type}/icons/{hash}`, that returns the SVG bytes addressed by content hash with `Content-Type: image/svg+xml`. The response MUST set `Cache-Control: public, max-age=31536000, immutable` and a strong `ETag` derived from the hash so HTTP caches (browser, reverse proxy, CDN) can serve repeated fetches without re-hitting the control plane. A request whose `{hash}` does not match the stored bytes for `{type}` MUST return 404. Authentication and authorization match the resource-type metadata endpoints (A-013).
- **FR-027**: When the control plane returns icon metadata for a registered resource type that has no icon attached (e.g. types that bypass the build-time icon-resolution step described in A-010 / A-014), it MUST substitute the embedded repository-wide default icon (FR-006) before responding. Specifically, the response's `iconHash` MUST be the well-known hash of the default icon, and the bytes addressed by that hash via FR-026 MUST be the default SVG. No API surface that returns a registered resource type may return a null or empty icon reference.
- **FR-028**: The icon contract defined by FR-013, FR-024, FR-025, and FR-026 (per-node `iconRef`, deduplicated `icons` map, `includeIcons` opt-in, content-hash endpoint) MUST apply to every API surface that returns an application graph or any graph-shaped depiction of an application's resources, including future surfaces that render graphs **before** a deployment runs (e.g. plan or preview visualizations) and **during** a deployment (e.g. in-progress or per-step graphs). New graph-producing endpoints MUST NOT invent a separate icon model — they reuse `iconRef` on each node and either honor `includeIcons` for inline bytes or rely on FR-026 for byte resolution. This requirement is forward-looking: no such pre-deployment or in-progress endpoint is required to ship as part of this feature, but when one is added it MUST conform to this contract rather than working around it.

#### Rendering surfaces

- **FR-015**: The dashboard MUST request the application graph with `includeIcons=true` and render the icon supplied for each node, with sensible sizing. The dashboard MAY alternatively fetch icons via FR-026's endpoint and rely on browser/CDN caching; either way the rendered icon MUST be the bytes addressed by the node's `iconRef`.
- **FR-016**: The dashboard MUST render a built-in fallback icon when an icon reference cannot be resolved (empty bytes, network failure, malformed SVG), so that no node ever renders without a visible icon.
- **FR-017**: The CLI's `rad app graph` MUST default to `includeIcons=false`. JSON output (`-o json`) carries `iconRef`/`iconHash` for each node but not the SVG bytes. Consumers who want bytes (rare; the CLI does not render SVG itself) MUST be able to opt in via an explicit flag (e.g. `--include-icons`) that sets `includeIcons=true` on the underlying request.

#### Validation

- **FR-018**: The contributing repository's CI MUST validate every sibling icon file is well-formed SVG and within the size cap.
- **FR-019**: The contributing repository's CI MUST validate that the repository-wide default icon exists and is valid.

### Non-Functional Requirements

- **NFR-001**: Icons MUST be SVG. Other formats are explicitly rejected.
- **NFR-002**: The maximum size for any single icon MUST be 32 KiB (32 768 bytes) to keep manifests, API payloads, and graph responses manageable. The cap MUST be enforced consistently in the build step (FR-005) and in the contributing-repo CI (FR-018, FR-019).
- **NFR-003**: The `icon` field on the resource type schema MUST be optional.
- **NFR-004**: The change MUST be additive on the wire — older clients that do not know about the field MUST continue to operate normally.
- **NFR-005**: The cross-repository dependency in which the Radius product build vendors a file from the contributing repository MUST be explicitly documented in the contributing repository so the dependency is not invisible to maintainers of either side.
- **NFR-006**: The dashboard's rendering of icons supplied by the API MUST be defensive against malformed content so a malformed icon cannot break the graph view.
- **NFR-007**: The dashboard MUST sanitize SVG content received from any API response or icon endpoint before injecting it into the DOM, to prevent script injection or other active-content attacks via a maliciously crafted icon.
- **NFR-008**: The icon endpoint (FR-026) MUST NOT require the control plane to recompute SVG bytes per request. Bytes for a given content hash are immutable by definition, so the endpoint MUST be servable from a precomputed map and MUST be safe to put behind a CDN.

### Key Entities *(include if feature involves data)*

- **Resource Type Definition**: The contributor-authored description of a resource type, hosted in the contributing repository and published into the control plane. Gains a new optional `icon` data element carrying the SVG content as text and a derived `iconHash` (SHA-256 of the canonical SVG text).
- **Sibling Icon File**: An SVG file placed next to a resource type definition file in the contributing repository. The source of truth for that type's icon; consumed only by the build step.
- **Default Icon**: A single SVG file at a well-known path in the contributing repository. Used by the build step whenever a resource type does not declare its own icon, and embedded into the Radius binaries so the control plane can always resolve an icon. Has its own well-known content hash that becomes the `iconHash` for every type that falls through to the default.
- **Icon Reference (`iconRef` / `iconHash`)**: The canonical addressing model for icons in API responses. A short string carrying a SHA-256 content hash that identifies a specific SVG payload. Multiple nodes or types referring to the same icon share a single reference; consumers resolve a reference to bytes either by reading the response's `icons` map (when `includeIcons=true`) or by fetching the dedicated icon endpoint (FR-026).
- **Application Graph Node**: A node in the application-graph response that represents a deployed resource. Gains an `iconRef` field that resolves to the type's icon (custom or default).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of resource types registered with a freshly built Radius control plane have an icon resolvable through the resource type API surfaces — custom where declared, default otherwise — with zero manual intervention by a Radius operator.
- **SC-002**: 100% of nodes in any application-graph response carry a resolvable `iconRef`, verifiable on a sample application that uses both a custom-icon and a no-icon resource type. With `includeIcons=true`, the `icons` map contains one entry per distinct hash referenced and zero duplicates.
- **SC-003**: A platform engineer can add a custom icon to an existing resource type by dropping a single SVG file into the type's folder and running the existing publish command, without editing the resource type definition file. Time to add an icon, end-to-end, is under 5 minutes once the SVG is in hand.
- **SC-004**: 100% of the resource types that ship in the built-in providers manifests render with an icon in the dashboard graph view on a fresh install, with no follow-up configuration.
- **SC-007**: 100% of the built-in `Applications.Core/*` resource types listed in FR-021 render with their **own custom icon** (not the repository-wide default and not the same as each other) in the dashboard graph view on a fresh install, verifiable by deploying an application that exercises each of those types.
- **SC-005**: A pull request to the contributing repository that introduces a malformed, oversized, or non-SVG icon file is rejected by CI with a single, actionable error message in the PR check output.
- **SC-006**: An existing client of any in-use preview API of the affected surfaces continues to function unchanged when the icon-related fields are added — verified by replaying a representative client request/response and confirming no schema-breaking change.
- **SC-008**: A graph response with N nodes that all share the same resource type carries exactly one entry in its `icons` map for that type's content hash (N → 1 deduplication), verifiable on a sample application with at least 50 same-type nodes.
- **SC-009**: The dashboard's repeated renders of the same graph fetch each unique icon's bytes from the icon endpoint (FR-026) at most once per browser session, verifiable by inspecting network traces and observing 304 / cache-hit behavior on subsequent loads.

## Out of Scope

- Animated icons or raster formats (PNG, JPEG, WebP). SVG only.
- Theming or dark-mode variants. A single icon per type for v1.
- Per-instance icon overrides on individual deployed resources. The icon is a property of the type, not the instance.
- Populating custom icons across the existing catalog of contributed resource types. This feature ships the mechanism, the default icon, and custom icons for the built-in types (including `Applications.Core/*`); per-type artwork for the long tail of contributed types is a follow-up.
- Native rendering of icons in the CLI's human-readable text graph output. The icon reference flows through the CLI's JSON output (without bytes by default per FR-017) so machine consumers can correlate them; ASCII rendering is a separate concern.

## Design alternatives considered

This section records significant design alternatives the spec evaluated, the chosen contract's rationale, and what evidence would justify revisiting any of them.

### Chosen contract: hybrid (hash-keyed references with opt-in inlining and a cacheable endpoint)

**Summary**: Every icon is addressed by a SHA-256 content hash. API responses carry references (`iconRef` / `iconHash`) for every node or type. The bytes are delivered through one of two mechanisms, selected per request:

- **Opt-in inline** (`includeIcons=true`): the response includes a top-level `icons` map keyed by hash, with at most one entry per distinct hash (FR-024). This is what the dashboard uses today.
- **Dedicated endpoint** (FR-026): `GET …/resourceTypes/{type}/icons/{hash}` returns the bytes with `Cache-Control: public, max-age=31536000, immutable` and a strong `ETag`. This is what the dashboard switches to once it wants cross-graph, cross-session caching, and what an embedder (docs site, third-party UI) would use natively.

**Why this shape**:

- **Solves the duplication problem.** A graph with hundreds of nodes of one type carries one entry in `icons`, not hundreds. Response size scales with the number of *distinct* icons, not the node count (A-022).
- **Doesn't punish CLI / scripting consumers.** `includeIcons` defaults to `false` (FR-025, FR-017), so `rad app graph -o json` and any other automated client never receives bytes they cannot render. The default response is small and references-only.
- **Preserves a self-contained option.** Consumers that want a single self-contained artifact (the dashboard's first paint, an air-gapped pipeline) opt in with `includeIcons=true` and get bytes inline without follow-up requests.
- **Enables proper HTTP caching for UIs.** The dashboard's repeated renders, navigations, and cross-application views fetch each unique icon at most once per browser session via FR-026 (SC-009). Bytes are content-addressed and immutable, so caches are safe forever.
- **Backward compatible.** Older clients ignore `iconRef`, `iconHash`, and `icons` fields, and never request `includeIcons=true` (NFR-004, FR-014).

### Pure inline (rejected)

**Alternative**: Embed SVG bytes verbatim on every node and every resource-type description, always.

**Why rejected**: Duplicates bytes for repeated types in a graph (the hundreds-of-same-type case). Forces CLI / scripting consumers to receive bytes they will never render. Provides no path for HTTP caching even for the dashboard. The 32 KiB per-icon cap (NFR-002) keeps the *worst* case bounded but does nothing about predictable, gratuitous duplication.

### Pure endpoint with URL-only metadata (rejected)

**Alternative**: Resource-type and graph responses carry only a URL or ID; consumers must always fetch bytes from a separate endpoint.

**Why rejected**:

- **N+1 round trips for any renderer**, even when the renderer wants exactly one self-contained artifact. The dashboard's first paint becomes 1 + N requests where N is the number of distinct icons in the graph; an air-gapped consumer simply can't get the bytes.
- **Eliminates the self-contained option** for any consumer that wants it (dashboards behind a flaky network, scripted exports, documentation generators).
- **Forces every consumer to handle partial failure** (icon endpoint down while metadata endpoint is up) for a feature that is fundamentally cosmetic.

The chosen hybrid keeps the endpoint as an option without making it mandatory.

### What would change the decision

The chosen contract is intentionally flexible, but these conditions would justify revisiting the defaults or adding new mechanisms:

- A realistic dashboard render pattern shows that even one-icon-per-distinct-type inline payloads dominate the graph response size. Mitigation: flip `includeIcons` default to require explicit opt-in for the dashboard too, relying solely on FR-026 + HTTP caching.
- Per-instance, per-theme, or per-locale icon variants enter scope (currently excluded by A-021). Mitigation: extend `iconRef` to carry a variant selector, or move bytes entirely behind FR-026 with query parameters.
- Authentication/authorization for icon bytes needs to diverge from authentication for resource-type metadata (e.g. icons need to be publicly fetchable while metadata stays gated). Mitigation: split FR-026 onto a separate plane or origin with its own auth model.
- A second first-party rendering surface ships that would benefit from cross-surface caching (a public docs site, an IDE extension). The endpoint contract is already correct for this; only the dashboard's choice of mechanism would need to update.
