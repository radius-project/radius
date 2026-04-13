// PR page graph injection orchestrator.
// Detects app.bicep in PR changed files, fetches graph artifacts from base/head,
// computes diff, and renders an interactive color-coded graph below PR description.

import type { StaticGraphArtifact } from '../shared/graph-types.js';
import { GraphGitHubAPI } from '../shared/github-api.js';
import { computeGraphDiff } from './graph-diff.js';
import { renderGraph, createDiffLegend } from './graph-renderer.js';

const GRAPH_CONTAINER_ID = 'radius-pr-graph-container';

/**
 * Initialize the PR graph visualization.
 * Called when a PR page is detected that modifies app.bicep.
 */
export async function initPRGraph(owner: string, repo: string, pullNumber: number): Promise<void> {
  // Prevent duplicate injection.
  if (document.getElementById(GRAPH_CONTAINER_ID)) return;

  // Get the auth token from extension storage.
  const token = await getAuthToken();
  if (!token) return;

  const api = new GraphGitHubAPI(token);

  // Only render on PRs that change the repository-root app.bicep file.
  const modifiesAppBicep = await api.pullRequestModifiesAppBicep(owner, repo, pullNumber);
  if (!modifiesAppBicep) return;

  // Find the injection point — below PR description.
  const discussionBucket = document.getElementById('discussion_bucket');
  if (!discussionBucket) return;

  // Create the graph container with loading state.
  const wrapper = document.createElement('div');
  wrapper.id = GRAPH_CONTAINER_ID;
  wrapper.className = 'radius-graph-container';
  wrapper.innerHTML = `
    <div class="radius-graph-loading">
      <div class="radius-graph-loading-spinner"></div>
      <span>Generating app graph...</span>
    </div>
  `;

  // Insert before the discussion_bucket content.
  discussionBucket.parentNode?.insertBefore(wrapper, discussionBucket);

  try {
    // Fetch PR details (base/head repos + refs).
    const prDetails = await api.fetchPRDetails(owner, repo, pullNumber);

    // Fetch graph artifacts from the orphan branch for both base and head source branches.
    const [baseArtifact, headArtifact] = await Promise.all([
      api.fetchGraphArtifact(prDetails.baseOwner, prDetails.baseRepo, prDetails.baseRef),
      api.fetchGraphArtifact(prDetails.headOwner, prDetails.headRepo, prDetails.headRef),
    ]);

    // If neither branch has a graph artifact, show a waiting message.
    if (!baseArtifact && !headArtifact) {
      wrapper.innerHTML = `
        <div class="radius-graph-message">
          Application graph not yet available — waiting for CI to build.
        </div>
      `;
      return;
    }

    // Compute diff between base and head graphs.
    const baseGraph = baseArtifact?.application ?? null;
    const headGraph = headArtifact?.application ?? null;
    const diff = computeGraphDiff(baseGraph, headGraph);

    // Get all resources to render (head resources + removed resources from base).
    const allResources = [
      ...(headGraph?.resources ?? []),
      ...diff.removed,
    ];

    if (allResources.length === 0) {
      wrapper.innerHTML = `
        <div class="radius-graph-message">
          No resources found in the application graph.
        </div>
      `;
      return;
    }

    // Clear loading state and render graph.
    wrapper.innerHTML = '';

    const graphCanvas = document.createElement('div');
    graphCanvas.className = 'radius-graph-canvas';
    wrapper.appendChild(graphCanvas);

    renderGraph({
      container: graphCanvas,
      resources: allResources,
      diff,
      context: {
        owner: prDetails.headOwner,
        repo: prDetails.headRepo,
        ref: prDetails.headRef,
        appFile: headArtifact?.sourceFile ?? 'app.bicep',
        pullNumber,
      },
    });

    // Add diff legend.
    wrapper.appendChild(createDiffLegend());
  } catch (error) {
    wrapper.innerHTML = `
      <div class="radius-graph-message">
        Failed to load application graph.
      </div>
    `;
    console.error('[Radius] PR graph error:', error);
  }
}

/**
 * Get the GitHub OAuth token from extension storage.
 */
async function getAuthToken(): Promise<string | null> {
  try {
    if (!chrome?.storage?.local) return null;
    const result = await chrome.storage.local.get('github_token');
    return result.github_token ?? null;
  } catch {
    return null;
  }
}
