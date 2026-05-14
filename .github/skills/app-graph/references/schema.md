# StaticGraphArtifact Schema

The HTML viewer consumes a `StaticGraphArtifact` JSON object as produced
by `rad graph build`. The shape mirrors `src/shared/graph-types.ts` in
`radius-project/github-extension`.

```ts
interface StaticGraphArtifact {
  version: string;                  // e.g. "1.0.0"
  generatedAt: string;              // ISO 8601 timestamp
  sourceFile: string;               // path to source Bicep file
  application: {
    resources: ApplicationGraphResource[];
  };
}

interface ApplicationGraphResource {
  id: string;                       // full resource ID
  name: string;                     // display name
  type: string;                     // e.g. "Applications.Core/containers"
  provisioningState: string;        // e.g. "Succeeded"
  connections: ApplicationGraphConnection[];
  outputResources: ApplicationGraphOutputResource[];
  codeReference?: string;           // "src/path/file.ts#L10"
  appDefinitionLine?: number;       // 1-based line in app.bicep
  diffHash?: string;                // hash of review-relevant fields
}

interface ApplicationGraphConnection {
  id: string;                       // target resource ID
  direction: 'Inbound' | 'Outbound';
}

interface ApplicationGraphOutputResource {
  id: string;
  name: string;
  type: string;                     // e.g. "apps/Deployment"
}
```

## Notes for the renderer

- Nodes are derived from `application.resources`.
- Edges are derived from each resource's `connections` where
  `direction === 'Outbound'`, and only when the connection's target `id`
  also exists in `application.resources`.
- `codeReference` is `path#L<n>` — split on `#L` to get path and line.
- `appDefinitionLine` is 1-based.
