// Dedicated modeled app graph page.
// Full-page graph container that renders the application graph from
// the radius-graph orphan branch with interactive navigation popups.

import { GraphGitHubAPI } from '../shared/github-api.js';
import { renderGraph } from './graph-renderer.js';

const PAGE_CONTAINER_ID = 'radius-app-graph-page';

/**
 * Initialize the dedicated application graph page.
 * Creates a full-page graph container and renders the graph.
 */
export async function initAppPage(owner: string, repo: string, _appName: string): Promise<void> {
  // Prevent duplicate injection.
  if (document.getElementById(PAGE_CONTAINER_ID)) return;

  const token = await getAuthToken();
  if (!token) return;

  const api = new GraphGitHubAPI(token);

  // Create full-page container.
  const container = document.createElement('div');
  container.id = PAGE_CONTAINER_ID;
  container.style.cssText = 'max-width: 1280px; margin: 0 auto; padding: 24px;';

  // Add heading.
  const heading = document.createElement('h2');
  heading.textContent = 'Application Graph';
  heading.style.cssText = 'margin-bottom: 16px; font-size: 20px; font-weight: 600;';
  container.appendChild(heading);

  // Graph wrapper.
  const graphWrapper = document.createElement('div');
  graphWrapper.className = 'radius-graph-container';
  graphWrapper.style.minHeight = '600px';
  graphWrapper.innerHTML = `
    <div class="radius-graph-loading">
      <div class="radius-graph-loading-spinner"></div>
      <span>Loading application graph...</span>
    </div>
  `;
  container.appendChild(graphWrapper);

  // Replace main content.
  const mainContent = document.querySelector('main, .repository-content');
  if (mainContent) {
    mainContent.innerHTML = '';
    mainContent.appendChild(container);
  } else {
    document.body.appendChild(container);
  }

  try {
    // Get current branch from URL or default to main.
    const ref = getCurrentBranch(owner, repo) ?? await getDefaultBranch(owner, repo);
    const artifact = await api.fetchGraphArtifact(owner, repo, ref);

    if (!artifact || artifact.application.resources.length === 0) {
      graphWrapper.innerHTML = `
        <div class="radius-graph-message">
          Application graph not yet available — ensure CI has run to generate the graph artifact.
        </div>
      `;
      return;
    }

    // Render full-size graph without diff coloring.
    graphWrapper.innerHTML = '';

    const graphCanvas = document.createElement('div');
    graphCanvas.className = 'radius-graph-canvas';
    graphCanvas.style.minHeight = '550px';
    graphWrapper.appendChild(graphCanvas);

    renderGraph({
      container: graphCanvas,
      resources: artifact.application.resources,
      diff: null,
      context: {
        owner,
        repo,
        ref,
        appFile: artifact.sourceFile,
      },
    });
  } catch (error) {
    graphWrapper.innerHTML = `
      <div class="radius-graph-message">
        Failed to load application graph.
      </div>
    `;
    console.error('[Radius] App page graph error:', error);
  }
}

function getCurrentBranch(owner: string, repo: string): string | null {
  const path = window.location.pathname;
  const treeMatch = path.match(new RegExp(`^/${owner}/${repo}/tree/(.+)$`));
  if (treeMatch) return treeMatch[1].split('/')[0];
  return null;
}

async function getDefaultBranch(owner: string, repo: string): Promise<string> {
  try {
    const token = await getAuthToken();
    if (!token) return 'main';

    const resp = await fetch(
      `https://api.github.com/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`,
      { headers: { Authorization: `token ${token}` } },
    );
    if (!resp.ok) return 'main';
    const data = await resp.json();
    return data.default_branch ?? 'main';
  } catch {
    return 'main';
  }
}

async function getAuthToken(): Promise<string | null> {
  try {
    if (!chrome?.storage?.local) return null;
    const result = await chrome.storage.local.get('github_token');
    return result.github_token ?? null;
  } catch {
    return null;
  }
}
