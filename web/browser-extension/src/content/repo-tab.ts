// Repo root tab injection.
// Injects an "Application graph" tab on repository root pages alongside
// README/License tabs. Renders the current-state graph without diff coloring.

import { getGitHubToken } from '../shared/api.js';
import { GraphGitHubAPI } from '../shared/github-api.js';
import { renderGraph } from './graph-renderer.js';

const TAB_ID = 'radius-graph-tab';
const PANEL_ID = 'radius-graph-tab-panel';

/**
 * Initialize the repo root "Application graph" tab.
 * Only injects if app.bicep exists in the repo.
 */
export async function initRepoTab(owner: string, repo: string): Promise<void> {
  // Prevent duplicate injection.
  if (document.getElementById(TAB_ID)) return;

  const token = await getAuthToken();

  const api = new GraphGitHubAPI(token);

  // Check if app.bicep exists in the repository.
  const hasAppBicep = await api.checkFileExists(owner, repo, 'app.bicep');
  if (!hasAppBicep) return;

  // Find the tab bar on the repo root page.
  const tabBar = document.querySelector(
    '.UnderlineNav-body, [role="tablist"], .js-repo-nav',
  );
  if (!tabBar) return;

  // Create the "Application graph" tab button.
  const tab = document.createElement('button');
  tab.id = TAB_ID;
  tab.className = 'radius-graph-tab UnderlineNav-item';
  tab.setAttribute('role', 'tab');
  tab.innerHTML = `
    <svg class="radius-graph-tab-icon" viewBox="0 0 16 16" width="16" height="16">
      <path fill="currentColor" d="M1.5 1.75V13.5h13.75a.75.75 0 010 1.5H.75a.75.75 0 01-.75-.75V1.75a.75.75 0 011.5 0zm14.28 2.53l-5.25 5.25a.75.75 0 01-1.06 0L7 7.06 4.28 9.78a.75.75 0 01-1.06-1.06l3.25-3.25a.75.75 0 011.06 0L10 8.94l4.72-4.72a.75.75 0 011.06 1.06z"/>
    </svg>
    <span>Application graph</span>
  `;
  tabBar.appendChild(tab);

  let panelCreated = false;

  tab.addEventListener('click', async () => {
    // Toggle the selected state.
    const isSelected = tab.classList.contains('selected');

    if (isSelected) {
      // Deselect: hide panel.
      tab.classList.remove('selected');
      const panel = document.getElementById(PANEL_ID);
      if (panel) panel.style.display = 'none';
      return;
    }

    // Select this tab.
    tab.classList.add('selected');

    if (!panelCreated) {
      panelCreated = true;
      await renderRepoGraph(owner, repo, api);
    } else {
      const panel = document.getElementById(PANEL_ID);
      if (panel) panel.style.display = 'block';
    }
  });
}

async function renderRepoGraph(owner: string, repo: string, api: GraphGitHubAPI): Promise<void> {
  // Create the panel.
  const panel = document.createElement('div');
  panel.id = PANEL_ID;
  panel.className = 'radius-graph-container';
  panel.innerHTML = `
    <div class="radius-graph-loading">
      <div class="radius-graph-loading-spinner"></div>
      <span>Loading application graph...</span>
    </div>
  `;

  // Insert below the repo content area.
  const repoContent = document.querySelector(
    '.repository-content, [data-turbo-frame="repo-content-turbo-frame"], main',
  );
  if (repoContent) {
    repoContent.prepend(panel);
  } else {
    document.body.appendChild(panel);
  }

  try {
    // Determine the default branch.
    const defaultBranch = await getDefaultBranch(owner, repo);
    const artifact = await api.fetchGraphArtifact(owner, repo, defaultBranch);

    if (!artifact || artifact.application.resources.length === 0) {
      panel.innerHTML = `
        <div class="radius-graph-message">
          Application graph not yet available — run CI to generate the graph artifact.
        </div>
      `;
      return;
    }

    // Render without diff coloring (all resources as unchanged).
    panel.innerHTML = '';

    const graphCanvas = document.createElement('div');
    graphCanvas.className = 'radius-graph-canvas';
    panel.appendChild(graphCanvas);

    renderGraph({
      container: graphCanvas,
      resources: artifact.application.resources,
      diff: null,
      context: {
        owner,
        repo,
        ref: defaultBranch,
        appFile: artifact.sourceFile,
      },
    });
  } catch (error) {
    panel.innerHTML = `
      <div class="radius-graph-message">
        Failed to load application graph.
      </div>
    `;
    console.error('[Radius] Repo tab graph error:', error);
  }
}

async function getDefaultBranch(owner: string, repo: string): Promise<string> {
  try {
    const token = await getAuthToken();

    const resp = await fetch(
      `https://api.github.com/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}`,
      token ? { headers: { Authorization: `token ${token}` } } : undefined,
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
    const token = await getGitHubToken();
    return token || null;
  } catch {
    return null;
  }
}
