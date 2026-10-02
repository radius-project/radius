# Topic: Extensible Application Graph Augmentation and Exchange

* **Author**: Will Tsai ([@willtsai](https://github.com/willtsai))

## Topic Summary

Radius needs a supported extensibility model that lets external systems contribute context to an application graph without changing or obscuring the graph produced by Radius. Examples include service catalogs adding ownership, cloud inventory systems correlating deployed resources, observability platforms attaching telemetry identities and health summaries, policy engines reporting evaluation results, and resiliency tools adding failure-domain relationships.

This feature introduces **graph augmentations**: versioned, namespaced, provenance-bearing overlays associated with an immutable Radius graph snapshot. Radius-produced nodes, identities, properties, and relationships remain canonical. An augmentation may annotate a canonical element and add provider-owned relationships between known elements. Adding external nodes is a separately declared and authorized provider capability. Consumers can request the canonical graph alone or a graph exchange bundle containing the canonical graph and applicable augmentations.

The exchange contract also supports the inverse integration. An external tool may import a Radius graph snapshot, correlate its own entities to stable Radius identifiers, and use the topology to enrich its native experience. For example, an observability platform can import a Radius graph to connect services, backing infrastructure, deployment state, and telemetry resources without Radius becoming a metrics or log store.

This specification addresses [ai-extensions#823](https://github.com/radius-project/ai-extensions/issues/823) and depends on the canonical graph contract proposed in [ai-extensions#815](https://github.com/radius-project/ai-extensions/issues/815) and the provenance model proposed in [ai-extensions#820](https://github.com/radius-project/ai-extensions/issues/820).

### Top level goals

* Preserve an immutable, clearly identifiable Radius-produced graph as the source of truth for Radius application topology.
* Define who may augment a graph and how an augmentation provider is registered, authenticated, authorized, and attributed.
* Provide a versioned interchange contract that external tools can import without depending on Canvas, dashboard, or renderer-specific data.
* Allow providers to add namespaced properties and provider-owned relationships, with separately gated support for external nodes, without replacing canonical identity or core relationships.
* Bind every augmentation to a specific graph snapshot and expose its source, observation time, freshness, and expiration.
* Make conflicts, unsupported extensions, stale data, partial authorization, and provider failures explicit to consumers.
* Support modeled, planned, deployed, and future graph kinds through one augmentation model.
* Demonstrate both integration directions:
  * an external provider augments a Radius graph; and
  * an external tool imports a Radius graph to enrich its own experience.

### Non-goals (out of scope)

* Allowing an external provider to overwrite Radius resource IDs, resource types, graph kind, deployment state, or Radius-produced relationships.
* Making external observations part of Radius deployment orchestration or dependency ordering.
* Replacing an observability, inventory, policy, catalog, or resiliency system as the source of its domain data.
* Copying unbounded logs, traces, metric time series, credentials, connection strings, or secrets into a graph.
* Defining a new graph visualization or requiring every Radius consumer to display augmentations.
* Supporting in-process plug-ins inside the Radius control plane in the first release.
* Automatically executing third-party code while Radius generates the canonical graph.
* Defining the canonical application graph schema itself; that work belongs to #815.

## Decision summary

| Question | Decision |
|---|---|
| Who may augment a graph? | An authenticated workload or user identity registered by a platform administrator as an augmentation provider and authorized for the target application/environment scope. Offline files may be composed locally but are untrusted until accepted by an authorized store or consumer. |
| When does augmentation occur? | After canonical graph generation. Providers read or receive a snapshot, produce an overlay, and publish it independently. Consumers compose overlays at read time. Generation-time plug-ins are deferred. |
| Is external data canonical? | No. The canonical Radius graph is immutable. External data remains a separately addressable overlay even when a consumer presents a composed view. |
| How are conflicts handled? | Radius-owned fields and relationships always win and cannot be replaced. Provider namespaces do not merge. Conflicting facts from different providers coexist with provenance and freshness so policy or the consuming tool can decide how to present them. |
| May extensions add nodes and edges? | Namespaced annotations and provider-owned relationships between known elements are baseline capabilities. External node creation requires a separately declared and authorized capability. External elements cannot impersonate canonical nodes or redefine canonical edges. |
| How is data persisted? | Canonical snapshots and augmentations have independent lifecycles. Augmentations are stored or transported as separate documents keyed by provider, graph snapshot, and revision. |
| How is data refreshed or expired? | Each augmentation records `observedAt` and either `expiresAt` or an explicit non-expiring classification. Expired data is excluded from the default composed view but remains distinguishable from missing or failed data. |

## User profile and challenges

### User persona(s)

**Platform engineer / Radius operator.** Configures Radius environments, approves integrations, grants provider scopes, and needs confidence that third-party data cannot change application identity or deployment behavior.

**Integration developer.** Builds an adapter for an observability platform, service catalog, cloud inventory, policy engine, or resiliency product. Needs a stable graph contract, validation rules, predictable compatibility, and actionable errors.

**Application developer or operator.** Uses Canvas, Radius Dashboard, CLI, or another tool to understand an application. Needs to know which facts came from Radius, which came from an external system, and whether those facts are current.

**Agent or automation author.** Retrieves topology through structured operations and must distinguish declared, inferred, observed, and externally contributed facts before proposing an action.

**External-tool owner.** Wants to import Radius topology and identifiers into an existing product rather than embedding that product into Radius.

### Challenge(s) faced by the user

Radius currently produces application graph data for CLI and UI experiences, but integrations do not have a supported way to add or exchange domain-specific context. An integration must either fork the schema, maintain a private merge format, modify a renderer, or infer topology from Bicep and deployment data.

These approaches create several risks:

* Radius and a third party can disagree without a way to preserve both facts and their provenance.
* A consumer cannot reliably tell whether a field is Radius-authored, inferred, observed, or externally supplied.
* Dynamic data can remain visible after it is stale.
* Provider-specific fields can collide with future Radius fields or another provider's fields.
* An integration may accidentally expose secrets or copy high-volume telemetry into a topology document.
* UI adapters become de facto contracts, which prevents CLI, agents, and independent products from sharing the same graph.
* External products cannot reliably correlate their entities with stable Radius nodes and graph snapshots.

### Positive user outcome

An authorized integration can consume a Radius graph snapshot, correlate its data, and publish a bounded overlay with a stable namespace, provenance, and freshness. Radius and external consumers can validate and exchange the result without losing the original Radius graph. Users and agents can inspect a composed view while retaining the ability to identify the owner and evidence for every contributed fact.

An observability tool can also import the Radius graph directly, map Radius resource IDs to service and infrastructure entities, and enrich its own service map, incident scope, or navigation without sending telemetry payloads back to Radius.

## Key scenarios

### Scenario 1: Add ownership and catalog context

A service catalog provider annotates Radius workload nodes with catalog entity references, owner, system, and lifecycle. The provider uses its own namespace. Radius identity and deployment fields remain unchanged, and users can navigate from a graph node to the catalog entry.

### Scenario 2: Correlate cloud inventory

An Azure Resource Graph provider reads a deployed Radius graph, matches `outputResources` to Azure resource IDs, and contributes inventory properties and externally owned relationships. A consumer can distinguish a current match, no match, insufficient permission, stale inventory, and provider failure.

### Scenario 3: Import Radius topology into an observability platform

An observability platform imports a Radius graph exchange bundle and maps canonical Radius nodes to OpenTelemetry service/resource identities or the platform's entity IDs. It uses Radius connections and output-resource relationships to enrich service maps, incident blast-radius analysis, and navigation. The observability platform remains authoritative for telemetry; Radius does not ingest raw traces, logs, or metric series.

### Scenario 4: Add bounded operational context

An observability provider annotates a workload with telemetry entity links, a health classification, the evaluation window, and a deep link to detailed telemetry. It may add an external telemetry-service node and a `observedBy` relationship. The augmentation includes a short expiration time so an old health classification cannot appear current.

### Scenario 5: Add policy and resiliency relationships

A policy engine adds a namespaced compliance result to a node. A resiliency tool adds externally owned failure-domain nodes and `sharesFailureDomainWith` relationships. Neither provider can replace the canonical dependency or connection edges used by Radius.

### Scenario 6: Consume unknown or unavailable extensions safely

A consumer receives an augmentation whose namespace or minor schema fields it does not understand. It preserves or ignores the unknown payload according to capability negotiation while continuing to use the canonical graph. Invalid, incompatible, unauthorized, or expired augmentations are reported explicitly and do not corrupt the canonical graph.

## Key dependencies and risks

**Canonical application graph contract (#815)** – Augmentations require stable node and edge identifiers, graph kind, application/environment scope, schema version, and snapshot identity. Augmentation implementation should not define a competing canonical model.

**Graph provenance (#820)** – Canonical and external facts need a shared way to describe source evidence. Augmentation provenance adds provider identity, observation time, and provider revision but should reuse the common evidence model.

**Graph inspection operations (#818)** – Agents and large-graph consumers need filtered retrieval. Inspection APIs must return the canonical contract plus augmentation metadata, not an interface-specific representation.

**Current graph producers** – Modeled graphs from `rad app graph <app.bicep>` and deployed graphs from the `getGraph` APIs must produce stable snapshot inputs. The current `ApplicationGraphResponse` and Canvas adapter are migration inputs, not the long-term extension boundary.

**Resource type evolution** – Community resource types and recipes can introduce unknown node/resource types. The contract must remain open to unknown types and cannot depend on a closed enum of types from `resource-types-contrib`.

**Trust confusion** – A visually composed graph could make an external assertion appear Radius-authored. Mitigation: preserve overlays as separate documents, require provenance, reserve Radius namespaces, and expose source/freshness in all structured responses.

**Secret or sensitive-data disclosure** – Resource properties and telemetry links may reveal credentials or sensitive topology. Mitigation: deny known secret-bearing fields, apply the same graph sanitization used by Radius producers, impose size/type allowlists, and document that providers publish references and summaries rather than raw telemetry.

**Identity mismatch** – Providers may correlate by mutable display name and attach data to the wrong node. Mitigation: require stable canonical IDs for annotations and canonical-edge endpoints; permit provider-owned correlation evidence and confidence without changing identity.

**Staleness and partial failure** – External systems update at different rates and may be partially authorized. Mitigation: independent provider status, observation and expiration times, snapshot binding, and explicit `partial`, `stale`, `expired`, `unauthorized`, and `failed` states.

**Graph size and latency** – Eager composition can make graph retrieval slow or unbounded. Mitigation: bounded payloads, pagination/filtering for inspection, provider-specific limits, asynchronous publication, and no synchronous provider calls on the canonical graph read path.

**Compatibility fragmentation** – Providers may bind to UI-specific shapes or undocumented properties. Mitigation: published JSON Schema, conformance tests, compatibility ranges, examples, and a provider capability manifest.

## Key assumptions to test and questions to answer

* **Stable identity is sufficient for correlation.** Prototype against Azure Resource Graph, a service catalog, and an OpenTelemetry-oriented observability system. Measure the percentage of nodes correlated by immutable ID, explicit external reference, and fallback evidence.
* **Post-generation augmentation meets initial needs.** Validate that asynchronous overlays support inventory, catalog, policy, resiliency, and telemetry scenarios without an in-process generation hook.
* **A snapshot-bound overlay can refresh cheaply.** Test whether providers can republish after graph changes without recomputing unrelated external data. Consider provider-side caching keyed by canonical element ID and snapshot digest.
* **External nodes and edges are necessary.** Validate the reference telemetry scenario, which requires a relationship from a canonical workload to an external telemetry entity, while keeping first-release relationship types bounded.
* **Consumers can use references instead of raw operational data.** Confirm that health summaries, evaluation windows, entity identifiers, and deep links support useful experiences without embedding time series or logs.
* **The canonical contract can represent all graph kinds.** Confirm modeled, planned, deployed, and diff workflows expose a snapshot ID/digest and stable element IDs.
* **Offline exchange is required.** Validate use in CI artifacts and Git-backed workflows where no persistent Radius control plane is available.
* **Open question: storage ownership.** Determine whether the first implementation persists overlays in Radius, in the AI extension, or in an artifact store. The wire contract and lifecycle semantics must remain storage-independent.
* **Open question: signing.** Determine whether authenticated publication plus immutable audit metadata is sufficient for the first release or whether offline augmentation documents require detached signatures.
* **Open question: retention.** Define default retention for expired revisions and audit events separately from their visibility in composed graph reads.

## Current state

Radius currently exposes a read-only application graph computed on demand from persisted resources. The architecture describes:

* `Applications.Core/applications/getGraph@2023-10-01-preview` for the existing deployed graph.
* `Radius.Core/applications/getGraph@2025-08-01-preview` for the newer graph with icon metadata and `Connection` versus `Dependency` edge kinds.
* `rad app graph -a <name>` for a deployed graph and `rad app graph <app.bicep>` for a modeled graph.
* JSON output based on `ApplicationGraphResponse`, with resources, connections, and output resources.
* Sanitization that omits container environment maps because they may contain credentials.

The graph is not stored in a graph database or materialized view. It is computed from Radius resource state. Radius also has a graph persistence abstraction and an archive implementation that stores complete `ApplicationGraphResponse` documents, while modeled graph workflows may persist `app-graph.json` as a local or CI artifact. Those mechanisms do not provide provider identity, snapshot parentage, TTL, revocation, or augmentation-specific lifecycle operations and therefore should not be treated as the overlay contract.

`ai-extensions` currently adapts `ApplicationGraphResponse` into a Canvas-specific resource model, rebuilds inbound edges, validates `diffHash`, and carries source references and icons. The adapter currently reduces connections to ID and direction, dropping the newer canonical edge `kind`; it must not similarly discard provenance or augmentation metadata during migration. This is a consumer implementation, not a safe extension contract.

Radius Dashboard advertises application graph visualization, but its current `RadiusApi` does not expose `getGraph` and its backend exposes no augmentation host. Dashboard should therefore be treated as a future consumer requiring explicit API/client/UI work, not as evidence of an existing graph extension contract.

`resource-types-contrib` demonstrates that Radius resource types and recipes are intentionally open-ended. Graph consumers therefore already need to tolerate unknown resource types; graph augmentation must preserve that extensibility rather than create a closed list.

There is no supported provider registration, overlay lifecycle, namespace policy, conflict model, or import/export bundle that addresses #823 today.

## Details of user problem

When I integrate Radius with an enterprise catalog, inventory, observability, policy, or resiliency system, I cannot safely add that system's context to the Radius application graph. I either modify a Radius or UI-specific schema, build an undocumented merged document, or duplicate topology inference in my integration.

I cannot prove which system supplied a fact, whether the fact applies to the graph I am viewing, or whether it is still current. If two systems disagree, I do not have a defined way to preserve both assertions. I also cannot add a useful external relationship without risking collision with Radius identities or dependency semantics.

When I want my product to consume Radius topology, I need a renderer-independent, versioned package with stable identities and relationship semantics. Without it, I must parse Bicep, scrape a UI, or depend on internal implementation details. This increases integration cost, creates security risk, and makes agent decisions less trustworthy.

## Desired user experience outcome

I can register or use an authorized augmentation provider, give it the minimum application/environment scope it needs, and publish a validated overlay for an exact Radius graph snapshot. I can see the provider, namespace, evidence, observation time, expiration, and status for every external contribution. Radius-produced identity and relationships remain intact.

I can retrieve or export the canonical graph alone, selected augmentations, or a graph exchange bundle. My external tool can import the same canonical graph, correlate it with its own entities, and use Radius topology in its native experience. If an augmentation is stale, incompatible, unauthorized, invalid, or unavailable, I receive an explicit status without losing access to the canonical graph.

### Detailed user experience

1. A platform administrator registers a provider namespace such as `io.opentelemetry` or `com.example.catalog`, binds it to a workload identity, and grants read/write scope for selected applications or environments.
2. The provider retrieves or receives a canonical graph snapshot containing its schema version, snapshot ID and digest, graph kind, scope, stable node and edge IDs, and provenance.
3. The provider correlates canonical elements with external entities and creates an augmentation document.
4. The provider validates the document locally against the published schema and capability rules.
5. The provider publishes the augmentation or supplies it as an offline artifact. Radius verifies identity, namespace ownership, scope, snapshot binding, size limits, reserved fields, references, compatibility, and freshness metadata.
6. Invalid contributions fail atomically with element-level, actionable errors. The previously accepted revision remains available.
7. A consumer requests:
   * canonical graph only;
   * canonical graph plus selected provider overlays; or
   * a portable graph exchange bundle.
8. The response reports each requested provider as `current`, `partial`, `stale`, `expired`, `unauthorized`, `incompatible`, `failed`, or `notAvailable`.
9. The consumer composes the view without mutating the canonical graph and displays provenance/freshness where external facts affect user or agent decisions.
10. An external tool may instead import the exchange bundle, retain Radius stable IDs as foreign keys, and enrich its own entity graph. It may publish a separate augmentation back to Radius, but import does not require round-tripping data.
11. When the canonical graph changes, prior overlays no longer appear as current for the new snapshot. Providers publish a new revision; consumers may inspect old revisions for audit or comparison.

## Proposed capability and contract

### Canonical graph snapshot

The augmentation model consumes the canonical contract from #815. At minimum, a snapshot must expose:

| Field | Purpose |
|---|---|
| `schemaVersion` | Version of the canonical graph contract. |
| `snapshot.id` | Opaque stable identifier for this generated graph snapshot. |
| `snapshot.digest` | Digest of canonical graph content used to prevent applying an overlay to different content. |
| `snapshot.createdAt` | Time Radius generated the snapshot. |
| `snapshot.kind` | Modeled, planned, deployed, diff, or a future open value. |
| `applicationId` / `environmentId` | Scope used for authorization and correlation. |
| Stable node and edge IDs | Targets for annotations and externally contributed relationships. |

### Augmentation document

An augmentation is a separate document with these logical sections:

| Section | Required behavior |
|---|---|
| `schemaVersion` | Versions the augmentation envelope independently from the canonical graph. |
| `provider` | Registered provider ID, namespace, implementation version, and authenticated publisher identity. |
| `compatibility` | Supported canonical graph schema range and declared capabilities. |
| `target` | Application/environment scope plus exact snapshot ID and digest. |
| `revision` | Provider-scoped immutable revision ID. Publishing the same revision is idempotent. |
| `observedAt` / `expiresAt` | Freshness and expiration. Non-expiring data must declare a static classification explicitly. |
| `status` | Complete or partial, including bounded diagnostics for omitted domains. |
| `annotations` | Namespaced properties targeting existing canonical or provider-owned element IDs. |
| `nodes` | Optional externally owned nodes with provider-namespaced IDs and types. Accepted only when provider registration grants the external-node capability. |
| `edges` | Optional externally owned relationships with stable IDs, open relationship types, source, target, and evidence. |
| `provenance` | Provider, source system, collection method, evidence references, and correlation confidence where inferred. |

### Namespace and identity rules

* Radius reserves its canonical field names, relationship types, and namespaces.
* Provider namespaces use a globally collision-resistant form such as a reverse DNS name.
* A registered namespace has one administrative owner and an allowlist of publisher identities.
* Provider-owned node, edge, annotation, and relationship type IDs are qualified by that namespace.
* An annotation targets an existing element by stable ID. It cannot patch an element by JSON path or replace the element.
* Provider-owned nodes cannot use an ID that could be interpreted as a Radius resource ID.
* An external edge may reference canonical or provider-owned nodes. Both endpoints must resolve within the exchange bundle or accepted augmentation set.
* External edges do not participate in Radius deployment ordering, connection injection, recipe execution, or lifecycle management.
* Provider annotations and ephemeral observations do not participate in the canonical `diffHash`. Provider-aware overlay comparison is a separate operation so telemetry changes cannot make application-definition diffs noisy.

### Conflict behavior

* Canonical Radius facts are never overwritten.
* Two providers may publish semantically conflicting facts because their properties remain in separate namespaces.
* A provider may supersede only its own prior revision for the same target snapshot.
* Within one provider revision, duplicate IDs or duplicate keys for the same target are validation errors.
* A consumer may apply a local presentation policy, but the exchange response retains all accepted facts and their provenance.
* Radius does not compute a universal "winner" between external providers.

### Trust and authorization

* Provider registration is an administrative operation.
* Publication requires authenticated identity, namespace ownership, and application/environment-scoped authorization.
* Read access to augmentations is no broader than access to the underlying canonical graph and may be narrower by provider policy.
* Server-derived audit fields identify the publisher and acceptance time; clients cannot forge them.
* Offline documents are marked unverified unless a consumer validates an accepted signature or trust record.
* Provider failure never changes the canonical graph response into a success-shaped merged result. The canonical graph and per-provider status are returned distinctly.

### Freshness and lifecycle

* Dynamic observations require an expiration time and a bounded validity interval.
* Static metadata may omit expiration only when declared `static`; it still records collection time and revision.
* The default composed view includes only augmentations that match the exact snapshot and are not expired.
* Consumers may request stale or expired revisions for audit, diagnostics, or historical comparison.
* A graph change does not silently carry an augmentation forward. Providers must publish for the new snapshot, though they may reuse cached correlation internally.
* Deleting or disabling a provider stops future publication and removes its overlays from default composition without altering canonical history.

### Import and export

The portable **graph exchange bundle** contains:

* one unchanged canonical graph snapshot;
* zero or more separate augmentation documents;
* validation and provider-status metadata; and
* no renderer-specific layout or UI state in the core contract.

Consumers can select:

* `canonical` – Radius-produced graph only;
* `bundle` – canonical graph and requested raw overlays; or
* `composed` – a convenience projection that still labels every external field and element with provider and provenance.

JSON is the required first format. The schema must allow deterministic canonicalization for digest verification. CLI/file output and service/API responses use the same contract.

For telemetry integrations, the recommended exchange is topology and identity rather than telemetry payloads:

* Radius exports stable workload, resource, connection, dependency, and output-resource identities.
* The telemetry tool records Radius IDs as external identifiers or entity attributes and maps them to its service/resource model.
* The tool may return bounded annotations such as entity URI, health classification, evaluation window, and `observedBy` relationships.
* Raw logs, spans, metric series, exemplars, and credentials remain in the telemetry system and are accessed through authorized deep links or native APIs.

### Compatibility behavior

* Major schema versions may make breaking changes and require explicit provider support.
* Minor versions add optional fields, statuses, capabilities, or open enum values. Consumers must ignore or preserve unknown optional fields.
* Unknown provider namespaces do not invalidate a bundle.
* Unknown external node or relationship types remain representable and inspectable.
* A provider declares the canonical graph versions it supports. An incompatible augmentation is retained as a diagnostic artifact but excluded from default composition.
* Canonical graph, augmentation envelope, and individual provider payload versions evolve independently.
* Conformance fixtures cover current, previous-minor, unknown-provider, unknown-type, stale, expired, partial, and incompatible cases.

## Key investments

### Feature 1: Versioned augmentation schema and conformance suite

Define JSON Schema and representative documents for annotations, external nodes, external relationships, provider status, provenance, freshness, validation errors, and the graph exchange bundle. Include deterministic digest rules and compatibility guidance.

Acceptance criteria:

* Every contribution identifies provider namespace, revision, applicable graph snapshot, observation time, and provenance.
* Unknown resource and relationship types are valid.
* Reserved-field replacement, canonical-ID impersonation, unresolved references, duplicate IDs, invalid namespaces, stale snapshot digests, and oversized payloads fail with actionable errors.
* Tests cover all completion expectations in #823.

### Feature 2: Provider registration, trust, and policy

Add a provider manifest and administrative registration flow that binds namespace, publisher identities, scopes, supported schema ranges, capabilities, payload limits, and freshness limits.

Acceptance criteria:

* Least-privilege read and publish roles can be scoped by application/environment and provider.
* Publisher and acceptance audit data are server-derived.
* Providers cannot publish outside their namespace or granted scope.
* Disabling a provider has explicit, documented read and retention behavior.

### Feature 3: Augmentation validation, persistence, and lifecycle

Implement a storage-independent service boundary for atomic publication, immutable revisions, current-revision selection, expiry, provider status, and audit retrieval. Do not synchronously call providers while serving the canonical graph.

Acceptance criteria:

* Failed publication leaves the previous accepted revision intact.
* Exact snapshot binding is enforced.
* Current, partial, stale, expired, unavailable, unauthorized, incompatible, and failed states are distinguishable.
* Retention and cleanup are configurable and observable.

### Feature 4: Graph import/export and inspection operations

Expose canonical, bundle, and composed retrieval through the appropriate Radius API, CLI JSON/file output, and agent inspection operations. Reuse one contract across these interfaces.

Acceptance criteria:

* An external tool can import a bundle without Canvas or Dashboard dependencies.
* Large graphs support filtering, provider selection, and bounded pagination where applicable.
* Consumers can retrieve raw overlays and verify the canonical snapshot digest.
* Existing Canvas and Dashboard graph consumers have a documented migration path.

### Feature 5: Reference augmentation provider

Build one provider that demonstrates:

* a namespaced property on a canonical workload;
* an externally owned node;
* an external relationship to the canonical workload;
* provenance, correlation evidence, freshness, expiration, and partial-failure behavior; and
* no raw telemetry ingestion.

An observability-oriented provider is preferred because it validates both enrichment directions: import Radius topology into a telemetry entity graph and optionally return bounded operational context to Radius.

### Feature 6: Consumer experience and provenance visibility

Update Radius Canvas and, where appropriate, Radius Dashboard to consume the common contract. Canonical-only behavior remains available. External facts show provider and freshness, and expired or unavailable providers do not masquerade as an empty successful result.

Acceptance criteria:

* Users can distinguish Radius-produced and external elements.
* Users can inspect provider, evidence, observation time, and expiration.
* A consumer that does not support augmentations continues to render the canonical graph.

## Success measures

* A reference provider passes the conformance suite and publishes one property, node, and relationship without modifying canonical graph bytes.
* A reference observability consumer imports a Radius bundle and correlates representative workload and output-resource identities.
* One canonical snapshot can be combined with multiple provider overlays without key collision.
* Invalid and unauthorized publications are rejected with actionable diagnostics.
* Provider unavailability adds no synchronous latency to canonical-only graph retrieval.
* Agents and UI consumers can distinguish canonical, external-current, external-stale, and external-unavailable data in structured responses.
* Existing modeled and deployed graph experiences continue to work without requiring augmentation providers.

## References

* [Issue #823: Define an extensibility model for application graph augmentation](https://github.com/radius-project/ai-extensions/issues/823)
* [Issue #815: Define a versioned application graph contract for agent consumption](https://github.com/radius-project/ai-extensions/issues/815)
* [Issue #820: Add provenance and source evidence to application graph elements](https://github.com/radius-project/ai-extensions/issues/820)
* [Issue #818: Expose agent-oriented application graph inspection operations](https://github.com/radius-project/ai-extensions/issues/818)
* [Issue #832: Application Graph as an Intelligence Layer](https://github.com/radius-project/ai-extensions/issues/832)
* [Radius application graph architecture](https://github.com/radius-project/radius/blob/main/docs/architecture/application-graph.md)
* [Application Graph Visualization design](https://github.com/radius-project/radius/blob/main/eng/design-notes/resources/2026-04-app-graph.md)
* [Radius feature specification template](https://github.com/radius-project/radius/blob/main/eng/design-notes/templates/feature-spec-template.md)
* [Radius CLI application graph reference](https://docs.radapp.io/reference/cli/rad_application_graph/)
* [Radius AI extension graph skill](https://github.com/radius-project/ai-extensions/blob/main/extensions/radius/skills/radius-app-graph/SKILL.md)
* [Radius Canvas graph adapter](https://github.com/radius-project/ai-extensions/blob/main/packages/core/src/graph/appgraph.ts)
* [Radius Dashboard](https://github.com/radius-project/dashboard)
* [Radius resource types and recipes contributions](https://github.com/radius-project/resource-types-contrib)
