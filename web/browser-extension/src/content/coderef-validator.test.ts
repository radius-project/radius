// Unit tests for codeReference security validation and URL construction.

import { isValidCodeReference, parseCodeReference, buildGitHubFileUrl } from './coderef-validator.js';

// Helper: simple test runner for environments without a test framework.
let passed = 0;
let failed = 0;

function assert(condition: boolean, msg: string) {
  if (condition) {
    passed++;
  } else {
    failed++;
    console.error(`FAIL: ${msg}`);
  }
}

function assertEqual<T>(actual: T, expected: T, msg: string) {
  assert(actual === expected, `${msg} — expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
}

async function run(): Promise<void> {

// --- isValidCodeReference tests ---

// Valid paths
assert(isValidCodeReference('src/app.ts'), 'simple path');
assert(isValidCodeReference('src/cache/redis.ts'), 'nested path');
assert(isValidCodeReference('lib/cache.go'), 'go file');
assert(isValidCodeReference('src/app.ts'), 'ts file');
assert(isValidCodeReference('README.md'), 'root file');
assert(isValidCodeReference('src/my-component/index.ts'), 'hyphenated dir');
assert(isValidCodeReference('src/my_component/index.ts'), 'underscored dir');

// Valid paths with line anchors
assert(isValidCodeReference('src/app.ts#L1'), 'line anchor L1');
assert(isValidCodeReference('src/app.ts#L42'), 'line anchor L42');
assert(isValidCodeReference('lib/cache.go#L100'), 'line anchor L100');

// Invalid: path traversal
assert(!isValidCodeReference('../secret/file.ts'), 'parent traversal');
assert(!isValidCodeReference('src/../../etc/passwd'), 'nested traversal');
assert(!isValidCodeReference('src/../other.ts'), 'mid-path traversal');

// Invalid: URL schemes
assert(!isValidCodeReference('https://example.com/file.ts'), 'https scheme');
assert(!isValidCodeReference('file:///tmp/secret'), 'file scheme');
assert(!isValidCodeReference('javascript:alert(1)'), 'javascript scheme');

// Invalid: absolute paths
assert(!isValidCodeReference('/etc/passwd'), 'absolute unix path');
assert(!isValidCodeReference('/src/app.ts'), 'absolute src path');

// Invalid: query strings
assert(!isValidCodeReference('src/app.ts?v=1'), 'query string');

// Invalid: backslashes
assert(!isValidCodeReference('src\\lib\\util.ts'), 'backslash path');

// Invalid: empty/null
assert(!isValidCodeReference(''), 'empty string');
assert(!isValidCodeReference(null), 'null');
assert(!isValidCodeReference(undefined), 'undefined');

// Invalid: special characters
assert(!isValidCodeReference('src/app.ts#42'), 'hash without L');
assert(!isValidCodeReference('src/app.ts#L'), 'hash L without number');
assert(!isValidCodeReference('src/app with spaces.ts'), 'spaces in path');

// --- parseCodeReference tests ---

assertEqual(parseCodeReference('src/app.ts').path, 'src/app.ts', 'parse path only');
assertEqual(parseCodeReference('src/app.ts').line, undefined, 'parse path only - no line');
assertEqual(parseCodeReference('src/app.ts#L42').path, 'src/app.ts', 'parse path with line');
assertEqual(parseCodeReference('src/app.ts#L42').line, 42, 'parse line number');
assertEqual(parseCodeReference('src/app.ts#L1').line, 1, 'parse line 1');

// --- buildGitHubFileUrl tests ---

assertEqual(
  await buildGitHubFileUrl({ owner: 'user', repo: 'repo', ref: 'main', path: 'src/app.ts' }),
  'https://github.com/user/repo/blob/main/src/app.ts',
  'basic blob URL',
);

assertEqual(
  await buildGitHubFileUrl({ owner: 'user', repo: 'repo', ref: 'main', path: 'src/app.ts', line: 42 }),
  'https://github.com/user/repo/blob/main/src/app.ts#L42',
  'blob URL with line',
);

assertEqual(
  await buildGitHubFileUrl({ owner: 'user', repo: 'repo', ref: 'feat/branch', path: 'src/app.ts' }),
  'https://github.com/user/repo/blob/feat%2Fbranch/src/app.ts',
  'URL encodes ref with slash',
);

assertEqual(
  await buildGitHubFileUrl({ owner: 'user', repo: 'repo', ref: 'main', path: 'src/app.ts', pullNumber: 123 }, true),
  'https://github.com/user/repo/pull/123/files#diff-841254fe75488c1bd4cd7f68f00b4be0e48dcfbc4a16b45847b68295e0e3b27b',
  'diff view URL',
);

// --- Summary ---
console.log(`\nResults: ${passed} passed, ${failed} failed`);
if (failed > 0) {
  process.exit(1);
}

}

run().catch((error) => {
  console.error(error);
  process.exit(1);
});
