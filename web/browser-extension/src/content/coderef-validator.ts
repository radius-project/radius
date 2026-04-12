// Security validation utilities for codeReference values and safe GitHub URL construction.
// Enforces strict allowlist regex per FR-009a/FR-009b to prevent path traversal,
// URL scheme injection, and other attacks via user-authored codeReference strings.

/**
 * Strict allowlist regex for codeReference values.
 * Allows: alphanumeric, underscores, hyphens, dots, forward slashes, optional #L<digits> anchor.
 * Rejects: everything else (URL schemes, query strings, backslashes, absolute paths, etc.)
 */
const CODE_REF_PATTERN = /^[a-zA-Z0-9_\-./]+(?:#L\d+)?$/;

/**
 * Validate a codeReference string against security rules.
 * Returns true only if the value is safe to use in URL construction.
 */
export function isValidCodeReference(value: string | undefined | null): boolean {
  if (!value || value.length === 0) return false;

  // Must match the allowlist pattern.
  if (!CODE_REF_PATTERN.test(value)) return false;

  // Must not contain path traversal segments.
  if (value.includes('..')) return false;

  // Must not start with / (absolute path).
  if (value.startsWith('/')) return false;

  // Must not contain URL schemes.
  if (value.includes('://')) return false;

  // Must not contain query strings.
  if (value.includes('?')) return false;

  // Must not contain backslashes.
  if (value.includes('\\')) return false;

  return true;
}

/**
 * Parse a validated codeReference into path and optional line number.
 */
export function parseCodeReference(value: string): { path: string; line?: number } {
  const hashIdx = value.indexOf('#L');
  if (hashIdx === -1) {
    return { path: value };
  }
  const path = value.substring(0, hashIdx);
  const line = parseInt(value.substring(hashIdx + 2), 10);
  return { path, line: isNaN(line) ? undefined : line };
}

export interface GitHubFileUrlParams {
  owner: string;
  repo: string;
  ref: string;
  path: string;
  line?: number;
  pullNumber?: number;
}

function toHex(bytes: Uint8Array): string {
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
}

async function buildDiffAnchor(path: string, line?: number): Promise<string> {
  const encodedPath = new TextEncoder().encode(path);
  const digest = await crypto.subtle.digest('SHA-256', encodedPath);
  const hash = toHex(new Uint8Array(digest));

  return line ? `diff-${hash}R${line}` : `diff-${hash}`;
}

/**
 * Build a safe GitHub file URL from validated path components.
 * Uses programmatic construction to avoid URL injection.
 *
 * @param params - The URL parts (owner, repo, ref, path, optional line).
 * @param diffView - If true, constructs a diff-view URL; otherwise a blob-view URL.
 * @returns The constructed GitHub URL string.
 */
export async function buildGitHubFileUrl(
  params: GitHubFileUrlParams,
  diffView: boolean = false,
): Promise<string> {
  const { owner, repo, ref, path, line, pullNumber } = params;

  // Encode each path component individually to prevent injection.
  const encodedOwner = encodeURIComponent(owner);
  const encodedRepo = encodeURIComponent(repo);
  const encodedRef = encodeURIComponent(ref);

  // Path segments are encoded individually to preserve directory structure.
  const encodedPath = path.split('/').map(encodeURIComponent).join('/');

  if (diffView && pullNumber != null) {
    const diffAnchor = await buildDiffAnchor(path, line);
    return `https://github.com/${encodedOwner}/${encodedRepo}/pull/${pullNumber}/files#${diffAnchor}`;
  }

  const url = `https://github.com/${encodedOwner}/${encodedRepo}/blob/${encodedRef}/${encodedPath}`;
  return line ? `${url}#L${line}` : url;
}
