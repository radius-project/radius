// Repo root tab injection.
// Injects an "Application graph" tab on repository root pages alongside
// README/License tabs. Renders the current-state graph without diff coloring.

import { getGitHubToken } from '../shared/api.js';
import { GraphGitHubAPI } from '../shared/github-api.js';
import { renderGraph } from './graph-renderer.js';

const TAB_ID = 'radius-graph-tab';
const PANEL_ID = 'radius-graph-tab-panel';

// Guard to prevent concurrent init attempts from MutationObserver retries.
let initInProgress = false;

/**
 * Initialize the repo root "Application graph" tab.
 * Only injects if app.bicep exists in the repo.
 */
export async function initRepoTab(owner: string, repo: string): Promise<void> {
  // Prevent duplicate injection.
  if (document.getElementById(TAB_ID)) return;
  // Prevent concurrent attempts.
  if (initInProgress) return;
  initInProgress = true;

  try {
    await doInitRepoTab(owner, repo);
  } finally {
    initInProgress = false;
  }
}

async function doInitRepoTab(owner: string, repo: string): Promise<void> {

  const token = await getAuthToken();

  const api = new GraphGitHubAPI(token);

  // Check if app.bicep exists in the repository.
  const hasAppBicep = await api.checkFileExists(owner, repo, 'app.bicep');
  if (!hasAppBicep) return;

  // GitHub's repo overview renders a Primer UnderlineNav for file tabs:
  //   <nav aria-label="Repository files">
  //     <ul role="list">
  //       <li><a aria-current="page">README</a></li>
  //     </ul>
  //   </nav>
  // The README markdown body is in a sibling div of the nav's parent.
  const tabList = document.querySelector(
    'nav[aria-label="Repository files"] ul[role="list"]',
  );
  if (!tabList) return;

  // Find the existing README tab item.
  const readmeTabItem = tabList.querySelector('li');
  const readmeTab = readmeTabItem?.querySelector('a');
  if (!readmeTab || !readmeTabItem) return;

  // Find the README body. Walk from the nav up to find the markdown content.
  const nav = tabList.closest('nav');
  const navParent = nav?.parentElement;
  const readmeBody = navParent?.parentElement?.querySelector(
    '.markdown-body, [data-hpc]',
  )?.closest('div') as HTMLElement | null;

  // Create our "Application graph" tab item matching GitHub's Primer structure.
  const graphTabItem = document.createElement('li');
  graphTabItem.className = readmeTabItem.className;

  const graphTabLink = document.createElement('a');
  graphTabLink.id = TAB_ID;
  graphTabLink.href = '#';
  graphTabLink.className = readmeTab.className;
  graphTabLink.removeAttribute('aria-current');
  graphTabLink.innerHTML = `
    <span data-component="icon">
      <svg aria-hidden="true" focusable="false" class="octicon" viewBox="0 0 16 16" width="16" height="16" fill="currentColor" style="display:inline-block;overflow:visible;vertical-align:text-bottom">
        <path d="M1.5 1.75V13.5h13.75a.75.75 0 010 1.5H.75a.75.75 0 01-.75-.75V1.75a.75.75 0 011.5 0zm14.28 2.53l-5.25 5.25a.75.75 0 01-1.06 0L7 7.06 4.28 9.78a.75.75 0 01-1.06-1.06l3.25-3.25a.75.75 0 011.06 0L10 8.94l4.72-4.72a.75.75 0 011.06 1.06z"/>
      </svg>
    </span>
    <span data-component="text">Application graph</span>
  `;

  graphTabItem.appendChild(graphTabLink);
  tabList.appendChild(graphTabItem);

  // Graph panel — hidden initially, inserted as a sibling of the README body.
  const graphPanel = document.createElement('div');
  graphPanel.id = PANEL_ID;
  graphPanel.className = 'radius-graph-container';
  graphPanel.style.display = 'none';
  if (readmeBody) {
    readmeBody.insertAdjacentElement('afterend', graphPanel);
  } else if (navParent?.parentElement) {
    navParent.parentElement.appendChild(graphPanel);
  }

  let graphLoaded = false;

  readmeTab.addEventListener('click', (e) => {
    e.preventDefault();
    readmeTab.setAttribute('aria-current', 'page');
    graphTabLink.removeAttribute('aria-current');
    if (readmeBody) readmeBody.style.display = '';
    graphPanel.style.display = 'none';
  });

  graphTabLink.addEventListener('click', async (e) => {
    e.preventDefault();
    graphTabLink.setAttribute('aria-current', 'page');
    readmeTab.removeAttribute('aria-current');
    if (readmeBody) readmeBody.style.display = 'none';
    graphPanel.style.display = '';

    if (!graphLoaded) {
      graphLoaded = true;
      await renderRepoGraph(owner, repo, api, graphPanel);
    }
  });
}

async function renderRepoGraph(owner: string, repo: string, api: GraphGitHubAPI, panel: HTMLElement): Promise<void> {
  panel.innerHTML = `
    <div class="radius-graph-loading">
      <div class="radius-graph-loading-spinner"></div>
      <span>Loading application graph...</span>
    </div>
  `;

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
