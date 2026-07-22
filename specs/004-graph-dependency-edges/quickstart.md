# Quickstart — Verify Application Graph Dependency Edges (Phase 1)

**Feature**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md) · **Date**: 2026-07-16

End-to-end verification against the `rabbitmq-app` fixture from Story 1 / Story 2 of the spec. Assumes the branch is built locally and `bicep` and `rad` are on `PATH`.

## Prerequisites

1. Radius repo built locally: `make build`.
2. Bicep CLI installed (`bicep --version`).
3. Fixtures cloned at `../my-radius-recipes/deploy/edges/` (sibling to this repo — see the spec's plain-text file paths).

## Steps

### 1. Compile the fixture

```bash
cd ../my-radius-recipes/deploy/edges
bicep build rabbitmq-app.bicep -o rabbitmq-app.json
```

Expected: `rabbitmq-app.json` is refreshed. Sanity check that both `dependsOn` blocks and the `connections.rabbitmq.source` expression appear as spec'd in [spec.md § Concrete example](spec.md#concrete-example--rabbitmq-app).

### 2. Build the static graph

```bash
cd -   # back to the radius repo
./bin/rad app graph -f ../my-radius-recipes/deploy/edges/rabbitmq-app.json > /tmp/static-graph-rabbit.json
```

### 3. Assert two graph nodes, zero excluded nodes

```bash
jq '.resources | length' /tmp/static-graph-rabbit.json
# Expected: 2

jq -r '.resources[].type' /tmp/static-graph-rabbit.json | sort
# Expected (exactly, no duplicates):
#   Radius.Compute/containers
#   Radius.Messaging/rabbitMQ

# Radius.Core/applications must NOT appear.
jq -r '.resources[].id' /tmp/static-graph-rabbit.json | grep -c 'Radius.Core/applications'
# Expected: 0
```

### 4. Assert exactly one edge between consumer and rabbitmq, tagged Connection

```bash
# Outbound edge on consumer.
jq -r '.resources[] | select(.name == "consumer") | .connections[]' /tmp/static-graph-rabbit.json
# Expected (single entry):
#   {
#     "direction": "Outbound",
#     "id": "/planes/radius/local/resourcegroups/default/providers/Radius.Messaging/rabbitMQ/rabbitmq",
#     "kind": "Connection"
#   }

# Mirrored inbound edge on rabbitmq.
jq -r '.resources[] | select(.name == "rabbitmq") | .connections[]' /tmp/static-graph-rabbit.json
# Expected (single entry):
#   {
#     "direction": "Inbound",
#     "id": "/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers/consumer",
#     "kind": "Connection"
#   }
```

Connection wins over the same-pair `Dependency` — this is SC-001.

### 5. Assert `dependsOn: ["app"]` produced no edges

```bash
# No edge on any node should target Radius.Core/applications/rabbitmq-app.
jq -r '[.resources[].connections[].id] | map(select(contains("Radius.Core/applications")))' /tmp/static-graph-rabbit.json
# Expected: []
```

This is SC-001's exclusion clause + Story 2 case #2.

### 6. Assert a `Dependency`-tagged edge appears when a `connections` block is absent

Create a variant fixture where `consumer` has no `properties.connections` block but still reads `rabbitmq.properties.secrets.name` via `secretKeyRef`. Bicep still emits `dependsOn: ["rabbitmq"]`. Build the static graph and assert:

```bash
jq -r '.resources[] | select(.name == "consumer") | .connections[]' /tmp/static-graph-rabbit-nodeps.json
# Expected:
#   { "direction": "Outbound", "id": "/…/rabbitMQ/rabbitmq", "kind": "Dependency" }

jq -r '.resources[] | select(.name == "rabbitmq") | .connections[]' /tmp/static-graph-rabbit-nodeps.json
# Expected:
#   { "direction": "Inbound", "id": "/…/containers/consumer", "kind": "Dependency" }
```

This is SC-002.

### 7. Confirm runtime graph is unchanged (Applications.Core) and gained `kind: Connection` (Radius.Core preview)

```bash
# Applications.Core existing test suite — byte-identical goldens.
go test ./pkg/corerp/frontend/controller/applications/... -count=1

# Radius.Core preview tests — golden files updated to include kind: Connection.
go test ./pkg/corerp/frontend/controller/applications/v20250801preview/... -count=1
```

Both green: SC-003.

## What "success" looks like

- The static graph output for `rabbitmq-app` has exactly 2 nodes and exactly 1 edge between them (mirrored on both sides), tagged `Connection`.
- The variant fixture with no `connections` block has exactly 1 edge tagged `Dependency`.
- All existing Applications.Core tests pass byte-identical to before.
- All Radius.Core preview tests pass; their goldens gained `kind: Connection` on every edge.

## Troubleshooting

- **Node count is 3 (still shows `rabbitmq-app`)** → the exclusion list didn't pick up `Radius.Core/applications`. Verify FR-005 membership in [pkg/cli/graph/modeled.go](../../pkg/cli/graph/modeled.go).
- **Two edges appear between `consumer` and `rabbitmq`** → Connection-wins de-dup is not firing. Verify FR-011 in [`pkg/graph/edges/edges.go`](../../pkg/graph/edges/edges.go).
- **`kind` is missing on runtime responses** → the Radius.Core preview converter isn't setting it. Verify FR-014 in the v20250801preview handler.
- **`Applications.Core` test golden files diff** → the shared internal builder in [graph_util.go](../../pkg/corerp/frontend/controller/applications/graph_util.go) changed. It must not (FR-016).
