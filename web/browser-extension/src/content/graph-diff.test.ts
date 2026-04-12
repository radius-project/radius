// Unit tests for graph diff computation.

import type {
  ApplicationGraphResource,
  ApplicationGraphResponse,
} from '../shared/graph-types.js';
import { computeGraphDiff, getResourceDiffStatus } from './graph-diff.js';

let passed = 0;
let failed = 0;

function assert(condition: boolean, msg: string) {
  if (condition) { passed++; } else { failed++; console.error(`FAIL: ${msg}`); }
}

function assertEqual<T>(actual: T, expected: T, msg: string) {
  assert(actual === expected, `${msg} — expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
}

function makeResource(id: string, diffHash?: string): ApplicationGraphResource {
  return {
    id,
    name: id.split('/').pop() ?? id,
    type: 'Applications.Core/containers',
    provisioningState: 'Succeeded',
    connections: [],
    outputResources: [],
    diffHash,
  };
}

// --- Added-only diff ---
{
  const base: ApplicationGraphResponse = { resources: [] };
  const head: ApplicationGraphResponse = { resources: [makeResource('/res/a', 'h1')] };
  const diff = computeGraphDiff(base, head);
  assertEqual(diff.added.length, 1, 'added-only: 1 added');
  assertEqual(diff.removed.length, 0, 'added-only: 0 removed');
  assertEqual(diff.modified.length, 0, 'added-only: 0 modified');
  assertEqual(diff.unchanged.length, 0, 'added-only: 0 unchanged');
}

// --- Removed-only diff ---
{
  const base: ApplicationGraphResponse = { resources: [makeResource('/res/a', 'h1')] };
  const head: ApplicationGraphResponse = { resources: [] };
  const diff = computeGraphDiff(base, head);
  assertEqual(diff.added.length, 0, 'removed-only: 0 added');
  assertEqual(diff.removed.length, 1, 'removed-only: 1 removed');
  assertEqual(diff.modified.length, 0, 'removed-only: 0 modified');
  assertEqual(diff.unchanged.length, 0, 'removed-only: 0 unchanged');
}

// --- Modified detection via diffHash ---
{
  const base: ApplicationGraphResponse = { resources: [makeResource('/res/a', 'hash_old')] };
  const head: ApplicationGraphResponse = { resources: [makeResource('/res/a', 'hash_new')] };
  const diff = computeGraphDiff(base, head);
  assertEqual(diff.modified.length, 1, 'modified: 1 modified');
  assertEqual(diff.modified[0].baseDiffHash, 'hash_old', 'modified: base hash');
  assertEqual(diff.modified[0].currentDiffHash, 'hash_new', 'modified: current hash');
}

// --- Unchanged pass-through ---
{
  const base: ApplicationGraphResponse = { resources: [makeResource('/res/a', 'same_hash')] };
  const head: ApplicationGraphResponse = { resources: [makeResource('/res/a', 'same_hash')] };
  const diff = computeGraphDiff(base, head);
  assertEqual(diff.unchanged.length, 1, 'unchanged: 1 unchanged');
  assertEqual(diff.modified.length, 0, 'unchanged: 0 modified');
}

// --- Empty graph handling ---
{
  const diff = computeGraphDiff(null, null);
  assertEqual(diff.added.length, 0, 'empty: 0 added');
  assertEqual(diff.removed.length, 0, 'empty: 0 removed');
  assertEqual(diff.modified.length, 0, 'empty: 0 modified');
  assertEqual(diff.unchanged.length, 0, 'empty: 0 unchanged');
}

// --- Mixed diff ---
{
  const base: ApplicationGraphResponse = {
    resources: [
      makeResource('/res/a', 'h1'),
      makeResource('/res/b', 'h2'),
      makeResource('/res/c', 'h3'),
    ],
  };
  const head: ApplicationGraphResponse = {
    resources: [
      makeResource('/res/a', 'h1'),      // unchanged
      makeResource('/res/b', 'h2_new'),   // modified
      makeResource('/res/d', 'h4'),       // added
    ],
  };
  const diff = computeGraphDiff(base, head);
  assertEqual(diff.unchanged.length, 1, 'mixed: 1 unchanged');
  assertEqual(diff.modified.length, 1, 'mixed: 1 modified');
  assertEqual(diff.added.length, 1, 'mixed: 1 added');
  assertEqual(diff.removed.length, 1, 'mixed: 1 removed (c)');

  // Test getResourceDiffStatus
  assertEqual(getResourceDiffStatus('/res/a', diff), 'unchanged', 'status: a unchanged');
  assertEqual(getResourceDiffStatus('/res/b', diff), 'modified', 'status: b modified');
  assertEqual(getResourceDiffStatus('/res/d', diff), 'added', 'status: d added');
  assertEqual(getResourceDiffStatus('/res/c', diff), 'removed', 'status: c removed');
  assertEqual(getResourceDiffStatus('/res/z', diff), 'unchanged', 'status: unknown = unchanged');
}

console.log(`\nResults: ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
