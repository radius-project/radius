// GitHub Contents API client for fetching graph artifacts and PR metadata.
// Separate from the existing GitHubClient to keep graph concerns isolated.

import type { StaticGraphArtifact } from './graph-types.js';

const GITHUB_API = 'https://api.github.com';

export interface PRDetails {
  baseOwner: string;
  baseRepo: string;
  baseRef: string;
  headOwner: string;
  headRepo: string;
  headRef: string;
}

export class GraphGitHubAPI {
  constructor(private token: string) {}

  /**
   * Fetch raw file contents from a specific branch.
   * Returns null if the file does not exist (404).
   */
  async getFileContents(owner: string, repo: string, path: string, ref: string): Promise<string | null> {
    const url = `${GITHUB_API}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/contents/${encodeURIComponent(path)}?ref=${encodeURIComponent(ref)}`;
    const resp = await fetch(url, {
      headers: {
        Accept: 'application/vnd.github.v3.raw',
        Authorization: `token ${this.token}`,
      },
    });

    if (resp.status === 404) return null;
    if (!resp.ok) throw new Error(`GitHub API error: ${resp.status} ${resp.statusText}`);
    return resp.text();
  }

  /**
   * Check if a file exists in the repository at the given ref.
   * Uses a HEAD request for efficiency.
   */
  async checkFileExists(owner: string, repo: string, path: string, ref?: string): Promise<boolean> {
    let url = `${GITHUB_API}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/contents/${encodeURIComponent(path)}`;
    if (ref) url += `?ref=${encodeURIComponent(ref)}`;

    const resp = await fetch(url, {
      method: 'HEAD',
      headers: {
        Authorization: `token ${this.token}`,
      },
    });

    return resp.ok;
  }

  /**
   * Fetch PR details including base/head repository and ref information.
   * Required for diff visualization on forked PRs.
   */
  async fetchPRDetails(owner: string, repo: string, pullNumber: number): Promise<PRDetails> {
    const url = `${GITHUB_API}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/pulls/${pullNumber}`;
    const resp = await fetch(url, {
      headers: {
        Accept: 'application/vnd.github.v3+json',
        Authorization: `token ${this.token}`,
      },
    });

    if (!resp.ok) throw new Error(`GitHub API error: ${resp.status} ${resp.statusText}`);

    const data = await resp.json();
    return {
      baseOwner: data.base.repo.owner.login,
      baseRepo: data.base.repo.name,
      baseRef: data.base.ref,
      headOwner: data.head.repo.owner.login,
      headRepo: data.head.repo.name,
      headRef: data.head.ref,
    };
  }

  /**
   * Fetch the static graph artifact from a specific branch.
   * Returns the parsed artifact or null if not found.
   */
  async fetchGraphArtifact(
    owner: string,
    repo: string,
    ref: string,
    artifactPath: string = '.radius/static/app.json',
  ): Promise<StaticGraphArtifact | null> {
    const raw = await this.getFileContents(owner, repo, artifactPath, ref);
    if (raw === null) return null;

    return JSON.parse(raw) as StaticGraphArtifact;
  }
}
