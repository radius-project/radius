// TypeScript type definitions for Application Graph visualization.
// These types mirror the Go ApplicationGraphResponse schema and add
// browser-extension-only types for diff computation and rendering.

/** Direction of a connection between two resources. */
export type Direction = 'Inbound' | 'Outbound';

/** Diff status for a resource when comparing base and head graphs. */
export type DiffStatus = 'added' | 'removed' | 'modified' | 'unchanged';

/** Deployment status for live infrastructure resources (P3). */
export type DeploymentStatus = 'queued' | 'in-progress' | 'success' | 'failed';

/** A connection between two resources in the application graph. */
export interface ApplicationGraphConnection {
  /** Resource ID of the connected resource. */
  id: string;
  /** Direction relative to the owning resource. */
  direction: Direction;
}

/** An output (infrastructure) resource generated from a Radius resource. */
export interface ApplicationGraphOutputResource {
  /** Infrastructure resource ID. */
  id: string;
  /** Infrastructure resource name. */
  name: string;
  /** Infrastructure resource type (e.g., `apps/Deployment`). */
  type: string;
}

/** A node in the application graph representing a single Radius resource. */
export interface ApplicationGraphResource {
  /** Full resource ID. */
  id: string;
  /** Resource display name. */
  name: string;
  /** Full resource type (e.g., `Applications.Core/containers`). */
  type: string;
  /** Provisioning state (e.g., `Succeeded`). */
  provisioningState: string;
  /** Inbound and outbound connections. */
  connections: ApplicationGraphConnection[];
  /** Underlying infrastructure resources. */
  outputResources: ApplicationGraphOutputResource[];
  /** Optional repo-root-relative file path to source code (e.g., `src/cache/redis.ts#L10`). */
  codeReference?: string;
  /** Optional 1-based line number of the resource declaration in app.bicep. */
  appDefinitionLine?: number;
  /** Optional stable hash of review-relevant properties for diff classification. */
  diffHash?: string;
}

/** Root response object for the application graph. */
export interface ApplicationGraphResponse {
  /** All resources in the application graph. */
  resources: ApplicationGraphResource[];
}

/** The CI-generated static graph JSON artifact stored on the radius-graph orphan branch. */
export interface StaticGraphArtifact {
  /** Schema version (e.g., `1.0.0`). */
  version: string;
  /** ISO 8601 timestamp of generation. */
  generatedAt: string;
  /** Path to the source Bicep file. */
  sourceFile: string;
  /** The graph data. */
  application: ApplicationGraphResponse;
}

/** A resource that exists in both branches but has changed properties. */
export interface ModifiedResource {
  /** Resource from the base branch. */
  base: ApplicationGraphResource;
  /** Resource from the PR/head branch. */
  current: ApplicationGraphResource;
  /** diffHash from the base branch artifact. */
  baseDiffHash: string;
  /** diffHash from the PR branch artifact. */
  currentDiffHash: string;
}

/** The computed difference between two application graphs. */
export interface GraphDiff {
  /** Resources present in head only. */
  added: ApplicationGraphResource[];
  /** Resources present in base only. */
  removed: ApplicationGraphResource[];
  /** Resources present in both but with changed diffHash. */
  modified: ModifiedResource[];
  /** Resources identical in both branches. */
  unchanged: ApplicationGraphResource[];
}
