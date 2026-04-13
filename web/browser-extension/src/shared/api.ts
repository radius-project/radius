// API client for the Radius GitHub integration.
// Calls the GitHub API directly from the extension — no separate backend server needed.

import { GitHubClient, GitHubAPIError } from './github-client.js';

export { GitHubClient, GitHubAPIError };
export { GitHubAPIError as APIError };

const STORAGE_KEY_GITHUB_TOKEN = 'radius_github_token';
const LEGACY_STORAGE_KEY_GITHUB_TOKEN = 'github_token';

// createClient returns a GitHubClient using the stored GitHub token.
// Returns null if no token is configured.
export async function createClient(): Promise<GitHubClient | null> {
  const token = await getGitHubToken();
  if (!token) return null;
  return new GitHubClient(token);
}

export async function getGitHubToken(): Promise<string> {
  const result = await chrome.storage.local.get([
    STORAGE_KEY_GITHUB_TOKEN,
    LEGACY_STORAGE_KEY_GITHUB_TOKEN,
  ]);

  const token = (result[STORAGE_KEY_GITHUB_TOKEN] as string)
    || (result[LEGACY_STORAGE_KEY_GITHUB_TOKEN] as string)
    || '';

  if (!result[STORAGE_KEY_GITHUB_TOKEN] && result[LEGACY_STORAGE_KEY_GITHUB_TOKEN]) {
    await chrome.storage.local.set({ [STORAGE_KEY_GITHUB_TOKEN]: token });
  }

  return token;
}

export async function setGitHubToken(token: string): Promise<void> {
  await chrome.storage.local.set({
    [STORAGE_KEY_GITHUB_TOKEN]: token,
    [LEGACY_STORAGE_KEY_GITHUB_TOKEN]: token,
  });
}
