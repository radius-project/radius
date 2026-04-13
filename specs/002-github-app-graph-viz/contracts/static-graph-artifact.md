# Contract: Static Graph JSON Artifact

**Version**: 1.0.0
**Produced by**: CI workflow (`build-app-graph.yaml`)
**Consumed by**: Browser extension (content script)
**Location**: `{source-branch}/app.json` on the `radius-graph` orphan branch

## Schema

```json
{
  "$schema": "https://radius-project.github.io/schemas/static-graph/v1.0.0.json",
  "version": "1.0.0",
  "generatedAt": "2026-04-12T10:30:00Z",
  "sourceFile": "app.bicep",
  "application": {
    "resources": [
      {
        "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/applications/myapp",
        "name": "myapp",
        "type": "Applications.Core/applications",
        "provisioningState": "Succeeded",
        "connections": [],
        "outputResources": []
      },
      {
        "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend",
        "name": "frontend",
        "type": "Applications.Core/containers",
        "provisioningState": "Succeeded",
        "codeReference": "src/frontend/server.ts#L1",
        "appDefinitionLine": 8,
        "diffHash": "sha256:2a6ec9e7f1f1...",
        "connections": [
          {
            "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/redisCaches/cache",
            "direction": "Outbound"
          }
        ],
        "outputResources": []
      },
      {
        "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/redisCaches/cache",
        "name": "cache",
        "type": "Applications.Datastores/redisCaches",
        "provisioningState": "Succeeded",
        "codeReference": "src/cache/redis.ts#L10",
        "appDefinitionLine": 18,
        "diffHash": "sha256:7b4301c1a4aa...",
        "connections": [
          {
            "id": "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/frontend",
            "direction": "Inbound"
          }
        ],
        "outputResources": []
      }
    ]
  }
}
```

## Field Descriptions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `version` | `string` | Yes | Schema version. Always `"1.0.0"` for initial release. |
| `generatedAt` | `string` | Yes | ISO 8601 UTC timestamp of when the artifact was generated. |
| `sourceFile` | `string` | Yes | Repository-root-relative path to the Bicep source file. |
| `application` | `object` | Yes | `ApplicationGraphResponse` object per existing schema. |
| `application.resources[]` | `array` | Yes | Array of `ApplicationGraphResource` objects. |
| `application.resources[].id` | `string` | Yes | Full Radius resource ID. |
| `application.resources[].name` | `string` | Yes | Display name of the resource. |
| `application.resources[].type` | `string` | Yes | Full resource type (without API version). |
| `application.resources[].provisioningState` | `string` | Yes | Always `"Succeeded"` for static graphs. |
| `application.resources[].codeReference` | `string` | No | Repo-root-relative file path with optional `#L<number>` anchor. |
| `application.resources[].appDefinitionLine` | `number` | No | 1-based line number of the resource declaration inside `app.bicep`. |
| `application.resources[].diffHash` | `string` | No | Opaque stable hash of review-relevant authorable properties. |
| `application.resources[].connections[]` | `array` | Yes | Array of connection objects. |
| `application.resources[].connections[].id` | `string` | Yes | Resource ID of the connected resource. |
| `application.resources[].connections[].direction` | `string` | Yes | `"Inbound"` or `"Outbound"`. |
| `application.resources[].outputResources[]` | `array` | Yes | Empty for static graphs (no deployment info). |

## Compatibility

- The `application` field is based on the existing `ApplicationGraphResponse` returned by the `getGraph` API endpoint, with additive fields (`codeReference`, `appDefinitionLine`, `diffHash`) required for static review scenarios.
- The `version`, `generatedAt`, and `sourceFile` fields are envelope metadata not present in the API response.

## Error Conditions

| Condition | Behavior |
|-----------|----------|
| Bicep compilation fails | CI workflow fails; no artifact is produced or committed |
| No resources in app definition | Artifact is produced with `application.resources: []` |
| Invalid `codeReference` value | Value is preserved in artifact; extension applies client-side validation before rendering |
| Line mapping fails for a resource declaration | Artifact is still produced; that resource omits `appDefinitionLine`, so the extension falls back to the file-level app definition link |
