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
  constructor(private token?: string | null) {}

  private createHeaders(headers: Record<string, string> = {}): Record<string, string> {
    if (this.token) {
      headers.Authorization = `token ${this.token}`;
    }

    return headers;
  }

  /**
   * Fetch raw file contents from a specific branch.
   * Returns null if the file does not exist (404).
   */
  async getFileContents(owner: string, repo: string, path: string, ref: string): Promise<string | null> {
    const encodedPath = path.split('/').map(encodeURIComponent).join('/');
    const url = `${GITHUB_API}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/contents/${encodedPath}?ref=${encodeURIComponent(ref)}`;
    const resp = await fetch(url, {
      headers: this.createHeaders({
        Accept: 'application/vnd.github.v3.raw',
      }),
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
    const encodedPath = path.split('/').map(encodeURIComponent).join('/');
    let url = `${GITHUB_API}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/contents/${encodedPath}`;
    if (ref) url += `?ref=${encodeURIComponent(ref)}`;

    const resp = await fetch(url, {
      method: 'HEAD',
      headers: this.createHeaders(),
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
      headers: this.createHeaders({
        Accept: 'application/vnd.github.v3+json',
      }),
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
   * Check whether a pull request modifies the repository-root app.bicep file.
   */
  async pullRequestModifiesAppBicep(owner: string, repo: string, pullNumber: number): Promise<boolean> {
    let page = 1;

    while (true) {
      const url = `${GITHUB_API}/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/pulls/${pullNumber}/files?per_page=100&page=${page}`;
      const resp = await fetch(url, {
        headers: this.createHeaders({
          Accept: 'application/vnd.github.v3+json',
        }),
      });

      if (!resp.ok) throw new Error(`GitHub API error: ${resp.status} ${resp.statusText}`);

      const files = (await resp.json()) as Array<{ filename?: string }>;
      if (files.some((file) => file.filename === 'app.bicep')) {
        return true;
      }

      if (files.length < 100) {
        return false;
      }

      page++;
    }
  }

  /**
   * Fetch the static graph artifact for a given source branch.
   * The artifact is stored on the orphan branch at {sourceBranch}/app.json.
   * Returns the parsed artifact or null if not found.
   */
  async fetchGraphArtifact(
    owner: string,
    repo: string,
    sourceBranch: string,
    orphanBranch: string = 'radius-graph',
  ): Promise<StaticGraphArtifact | null> {
    const artifactPath = `${sourceBranch}/app.json`;
    const raw = await this.getFileContents(owner, repo, artifactPath, orphanBranch);
    if (raw === null) return null;

    return JSON.parse(raw) as StaticGraphArtifact;
  }
}
