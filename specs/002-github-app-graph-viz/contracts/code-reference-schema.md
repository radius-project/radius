# Contract: codeReference Property Schema Extension

**Version**: 1.0.0
**Scope**: All Radius resource types (Applications.Core/*, Applications.Dapr/*, Applications.Datastores/*, Applications.Messaging/*)

## TypeSpec Definition

The `codeReference` property is added to the shared authorable resource-property bases in the TypeSpec definitions so Bicep authors can set it directly on resources. It is then propagated into the `ApplicationGraphResource` read model for graph rendering.

### Addition to shared authorable resource bases

```typespec
// typespec/radius/v1/resources.tsp
@doc("Optional repository-root-relative file path to the source code for this resource. Format: 'path/to/file.ext' or 'path/to/file.ext#L10'. Must use forward slashes, must not contain URL schemes, query strings, absolute paths, or path traversal segments.")
codeReference?: string;
```

### Addition to the graph read model

```typespec
// typespec/Applications.Core/applications.tsp
codeReference?: string;
appDefinitionLine?: int32;
diffHash?: string;
```

### Bicep Usage

```bicep
resource cache 'Applications.Datastores/redisCaches@2023-10-01-preview' = {
  name: 'cache'
  properties: {
    application: app.id
    codeReference: 'src/cache/redis.ts#L10'
    // ... other properties
  }
}
```

## Validation Rules

| Rule | Regex / Check | Example Valid | Example Invalid |
|------|--------------|---------------|-----------------|
| Format | `^[a-zA-Z0-9_\-./]+(?:#L\d+)?$` | `src/app.ts`, `lib/cache.go#L42` | `https://example.com/file.ts` |
| No path traversal | Must not contain `..` | `src/lib/util.ts` | `../secret/file.ts` |
| No absolute paths | Must not start with `/` | `src/main.go` | `/etc/passwd` |
| No URL schemes | Must not contain `://` | `src/api/handler.ts` | `file:///tmp/secret` |
| No query strings | Must not contain `?` | `src/app.ts#L10` | `src/app.ts?v=1` |
| Forward slashes only | Must use `/` not `\` | `src/lib/util.ts` | `src\lib\util.ts` |
| Line anchor format | `#L` followed by digits | `src/app.ts#L42` | `src/app.ts#42`, `src/app.ts#L` |

## Where Validation Occurs

| Component | Validation | Action on Invalid |
|-----------|-----------|-------------------|
| TypeSpec/API | None (optional string) | Accepted as-is |
| Static graph builder | Structural only (copies authorable value into read model) | Invalid values still emitted but treated as opaque strings |
| Browser extension | Full regex + traversal check | Silently omit "Source code" link |

**Design rationale**: Validation is enforced at the **rendering boundary** (browser extension) rather than at the API/storage level. This follows the principle of being liberal in what you accept and conservative in what you produce. The extension is the only component that constructs URLs from this value, so it is the appropriate place to enforce security validation.

## Go Model Impact

After TypeSpec regeneration, shared authorable properties and the graph read model gain:

```go
type ApplicationGraphResource struct {
    // ... existing fields ...

    // Optional repository-root-relative file path to the source code for this resource.
    CodeReference *string `json:"codeReference,omitempty"`

  // Optional 1-based line number in app.bicep for app-definition links.
  AppDefinitionLine *int32 `json:"appDefinitionLine,omitempty"`

  // Optional stable hash for modified-resource classification.
  DiffHash *string `json:"diffHash,omitempty"`
}
```

## Backward Compatibility

- **API consumers**: The field is optional with `omitempty` JSON tag. Existing consumers that don't know about `codeReference` will ignore it.
- **Bicep authors**: The property is optional. Existing `.bicep` files without `codeReference` continue to work unchanged.
- **Graph visualization**: Resources without `codeReference` show only the "App definition" link in the popup (no "Source code" link).
