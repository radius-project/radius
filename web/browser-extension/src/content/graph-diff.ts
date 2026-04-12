// Client-side graph diff computation.
// Builds resource ID maps from base and head ApplicationGraphResponse,
// classifies each resource as added, removed, modified, or unchanged.

import type {
  ApplicationGraphResource,
  ApplicationGraphResponse,
  GraphDiff,
  ModifiedResource,
} from '../shared/graph-types.js';

/**
 * Compute the diff between base (old) and head (new) application graphs.
 *
 * Classification rules:
 * - added:     resource ID in head but not in base
 * - removed:   resource ID in base but not in head
 * - modified:  resource ID in both but diffHash differs
 * - unchanged: resource ID in both with same diffHash
 */
export function computeGraphDiff(
  base: ApplicationGraphResponse | null,
  head: ApplicationGraphResponse | null,
): GraphDiff {
  const diff: GraphDiff = {
    added: [],
    removed: [],
    modified: [],
    unchanged: [],
  };

  const baseResources = base?.resources ?? [];
  const headResources = head?.resources ?? [];

  // Build lookup maps by resource ID.
  const baseMap = new Map<string, ApplicationGraphResource>();
  for (const r of baseResources) {
    baseMap.set(r.id, r);
  }

  const headMap = new Map<string, ApplicationGraphResource>();
  for (const r of headResources) {
    headMap.set(r.id, r);
  }

  // Process head resources: find added, modified, unchanged.
  for (const [id, headRes] of headMap) {
    const baseRes = baseMap.get(id);
    if (!baseRes) {
      diff.added.push(headRes);
    } else if (headRes.diffHash !== baseRes.diffHash) {
      const modified: ModifiedResource = {
        base: baseRes,
        current: headRes,
        baseDiffHash: baseRes.diffHash ?? '',
        currentDiffHash: headRes.diffHash ?? '',
      };
      diff.modified.push(modified);
    } else {
      diff.unchanged.push(headRes);
    }
  }

  // Process base resources: find removed.
  for (const [id, baseRes] of baseMap) {
    if (!headMap.has(id)) {
      diff.removed.push(baseRes);
    }
  }

  return diff;
}

/**
 * Get the diff status for a specific resource by ID.
 */
export function getResourceDiffStatus(
  resourceId: string,
  diff: GraphDiff,
): 'added' | 'removed' | 'modified' | 'unchanged' {
  if (diff.added.some((r) => r.id === resourceId)) return 'added';
  if (diff.removed.some((r) => r.id === resourceId)) return 'removed';
  if (diff.modified.some((m) => m.current.id === resourceId)) return 'modified';
  return 'unchanged';
}
