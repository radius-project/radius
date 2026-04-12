// Content script: injects Radius features into GitHub pages.
// - "Deploy with Radius" button on repo root pages
// - Application graph visualization on PR pages
// - "Application graph" tab on repo root pages
// Runs on github.com/{owner}/{repo} pages at document_idle.

import { initPRGraph } from './pr-graph.js';
import { initRepoTab } from './repo-tab.js';
import { initAppPage } from './app-page.js';

// ── Page Detection ────────────────────────────────────────────

interface PageInfo {
  type: 'pr' | 'repo-root' | 'app-page' | 'other';
  owner: string;
  repo: string;
  pullNumber?: number;
  appName?: string;
}

function detectPage(): PageInfo {
  const path = window.location.pathname;

  // PR page: /:owner/:repo/pull/:number
  const prMatch = path.match(/^\/([^/]+)\/([^/]+)\/pull\/(\d+)/);
  if (prMatch) {
    return { type: 'pr', owner: prMatch[1], repo: prMatch[2], pullNumber: parseInt(prMatch[3], 10) };
  }

  // Dedicated app page: /:owner/:repo/radius/app/:name
  const appMatch = path.match(/^\/([^/]+)\/([^/]+)\/radius\/app\/([^/]+)/);
  if (appMatch) {
    return { type: 'app-page', owner: appMatch[1], repo: appMatch[2], appName: appMatch[3] };
  }

  // Repo root: /:owner/:repo (optionally /tree/...)
  const repoMatch = path.match(/^\/([^/]+)\/([^/]+)(\/tree\/.*)?$/);
  if (repoMatch) {
    const owner = repoMatch[1];
    const reserved = ['settings', 'marketplace', 'explore', 'notifications', 'new', 'login', 'signup', 'orgs', 'topics'];
    if (!reserved.includes(owner)) {
      return { type: 'repo-root', owner, repo: repoMatch[2] };
    }
  }

  return { type: 'other', owner: '', repo: '' };
}

// ── Graph Feature Injection ───────────────────────────────────

let lastInjectedPath = '';

function injectGraphFeatures(): void {
  const path = window.location.pathname;
  if (path === lastInjectedPath) return;
  lastInjectedPath = path;

  const page = detectPage();

  switch (page.type) {
    case 'pr':
      if (page.pullNumber) {
        initPRGraph(page.owner, page.repo, page.pullNumber);
      }
      break;
    case 'repo-root':
      initRepoTab(page.owner, page.repo);
      break;
    case 'app-page':
      if (page.appName) {
        initAppPage(page.owner, page.repo, page.appName);
      }
      break;
  }
}

// ── Debounce utility ──────────────────────────────────────────

function debounce(fn: () => void, delay: number): () => void {
  let timer: ReturnType<typeof setTimeout>;
  return () => {
    clearTimeout(timer);
    timer = setTimeout(fn, delay);
  };
}

const debouncedInject = debounce(() => {
  injectGraphFeatures();
  if (!document.getElementById('radius-deploy-btn')) {
    injectRadiusButton();
  }
}, 300);


function injectRadiusButton(): boolean {
  // Only inject on the repository main page (root or code tab).
  const path = window.location.pathname;
  const match = path.match(/^\/([^/]+)\/([^/]+)(\/tree\/.*)?$/);
  if (!match) return false;

  const owner = match[1];
  const repo = match[2];

  // Don't inject on special GitHub pages.
  const reserved = ['settings', 'marketplace', 'explore', 'notifications', 'new', 'login', 'signup', 'orgs', 'topics'];
  if (reserved.includes(owner)) return false;

  // Don't inject on sub-pages (issues, pulls, actions, settings, etc.).
  const subPages = ['issues', 'pulls', 'actions', 'settings', 'projects', 'wiki', 'security', 'pulse', 'graphs', 'network', 'community', 'discussions', 'compare', 'commit', 'commits', 'branches', 'tags', 'releases', 'packages', 'deployments', 'codespaces'];
  const thirdSegment = path.split('/')[3];
  if (thirdSegment && !['tree', 'blob'].includes(thirdSegment) && subPages.includes(thirdSegment)) return false;

  // Check if already injected (prevents duplicates on SPA navigation).
  if (document.getElementById('radius-deploy-btn')) return true;

  // Only inject if the green "Code" button is present (confirms we're on the code view).
  let codeButton: HTMLElement | null = null;
  const primaryButtons = document.querySelectorAll<HTMLElement>('button[data-variant="primary"]');
  for (const btn of primaryButtons) {
    const label = btn.querySelector('[data-component="text"]');
    if (label && label.textContent?.trim() === 'Code') {
      codeButton = btn;
      break;
    }
  }

  // No Code button = not on the main code page. Don't inject.
  if (!codeButton) return false;

  // Create the button.
  const btn = document.createElement('button');
  btn.id = 'radius-deploy-btn';
  btn.className = 'radius-deploy-btn';
  btn.title = 'Setup with Radius — configure cloud environments';
  btn.innerHTML = `
    <svg viewBox="0 0 16 16" width="16" height="16" class="radius-deploy-icon">
      <path fill="currentColor" d="M8 0a8 8 0 110 16A8 8 0 018 0zm.75 4.75a.75.75 0 00-1.5 0v2.5h-2.5a.75.75 0 000 1.5h2.5v2.5a.75.75 0 001.5 0v-2.5h2.5a.75.75 0 000-1.5h-2.5v-2.5z"/>
    </svg>
    <span>Deploy</span>
  `;

  // Create dropdown menu (hidden by default).
  const dropdown = document.createElement('div');
  dropdown.id = 'radius-dropdown';
  dropdown.className = 'radius-dropdown hidden';
  // Initial state: prompt to define an application.
  dropdown.innerHTML = `
    <div class="radius-dropdown-hint">An application definition must be created prior to deploying.</div>
    <button class="radius-dropdown-item" data-action="define-app">+ Define an Application</button>
  `;

  btn.addEventListener('click', (e) => {
    e.preventDefault();
    e.stopPropagation();
    dropdown.classList.toggle('hidden');

    // Flip above if not enough space below.
    if (!dropdown.classList.contains('hidden')) {
      const btnRect = btn.getBoundingClientRect();
      const spaceBelow = window.innerHeight - btnRect.bottom;
      dropdown.classList.toggle('radius-dropdown--above', spaceBelow < 200);
    }
  });

  // Dropdown item handlers.
  dropdown.addEventListener('click', (e) => {
    const target = (e.target as HTMLElement).closest('[data-action]') as HTMLElement | null;
    if (!target) return;
    e.stopPropagation();
    dropdown.classList.add('hidden');

    const action = target.dataset.action;
    try {
      if (!chrome?.runtime?.id) return;
      if (action === 'define-app') {
        chrome.runtime.sendMessage({
          type: 'OPEN_TAB',
          owner,
          repo,
          step: 'app',
        }).catch(() => {});
      } else if (action === 'create-env-aws') {
        chrome.runtime.sendMessage({
          type: 'OPEN_TAB',
          owner,
          repo,
          step: 'env',
          provider: 'aws',
        }).catch(() => {});
      } else if (action === 'create-env-azure') {
        chrome.runtime.sendMessage({
          type: 'OPEN_TAB',
          owner,
          repo,
          step: 'env',
          provider: 'azure',
        }).catch(() => {});
      } else if (action === 'deploy-app') {
        chrome.runtime.sendMessage({
          type: 'OPEN_TAB',
          owner,
          repo,
          step: 'deploy',
        }).catch(() => {});
      }
    } catch {
      // Extension context invalidated.
    }
  });

  // Close dropdown when clicking outside.
  document.addEventListener('click', () => {
    dropdown.classList.add('hidden');
  });

  // Wrap button + dropdown in a container.
  const container = document.createElement('div');
  container.className = 'radius-deploy-container';
  container.appendChild(btn);
  container.appendChild(dropdown);

  // Insert directly after the Code button.
  codeButton.insertAdjacentElement('afterend', container);

  // Check environment status to show a badge.
  checkEnvironmentStatus(btn, owner, repo);

  // Inject the applications sidebar widget.
  void injectApplicationsSidebar(owner, repo);

  return true;
}

async function injectApplicationsSidebar(owner: string, repo: string): Promise<void> {
  if (document.getElementById('radius-applications-sidebar')) return;
  if (!chrome?.runtime?.id) return;

  const appInfo = await chrome.runtime.sendMessage({
    type: 'CHECK_APP',
    owner,
    repo,
  }) as { exists: boolean; filename?: string } | null;

  if (!appInfo?.exists) return;

  const appFile = appInfo.filename ?? 'app.bicep';
  const appName = appFile.replace(/\.bicep$/, '').replace(/^.*\//, '');
  const modeledAppURL = `https://github.com/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/radius/app/${encodeURIComponent(appName)}`;

  // Find the right sidebar on the repo page.
  const sidebar = document.querySelector(
    '.Layout-sidebar .BorderGrid, ' +
    '[class*="sidebar"] .BorderGrid, ' +
    '.repository-content .BorderGrid'
  );
  if (!sidebar) return;

  const widget = document.createElement('div');
  widget.id = 'radius-applications-sidebar';
  widget.className = 'BorderGrid-row';
  widget.innerHTML = `
    <div class="BorderGrid-cell">
      <h2 class="h4 mb-3">
        <a href="${modeledAppURL}" class="Link--primary no-underline" id="radius-applications-title">
          Radius Applications
        </a>
      </h2>
      <div class="radius-deployments-list">
        <a href="${modeledAppURL}" class="radius-deployment-item">
          <span class="radius-deployment-status"><span class="radius-status-queued">○</span></span>
          <span class="radius-deployment-info">
            <span class="radius-deployment-label">${escapeHTML(appName)}</span>
            <span class="radius-deployment-time">Modeled application graph</span>
          </span>
        </a>
      </div>
      <hr class="mt-3 mb-3">
      <h3 class="h5 mb-2">Recent deployments</h3>
      <div id="radius-deployments-list" class="radius-deployments-list">
        <span class="radius-deployments-loading">Loading...</span>
      </div>
    </div>
  `;

  sidebar.appendChild(widget);

  try {
    const deployments = await chrome.runtime.sendMessage({
      type: 'LIST_DEPLOYMENTS',
      owner,
      repo,
    }) as Array<{
      id: number;
      status: string;
      conclusion: string;
      htmlURL: string;
      createdAt: string;
      headBranch?: string;
      appFile?: string;
    }> | null;

    const listEl = document.getElementById('radius-deployments-list');
    if (!listEl) return;

    if (!deployments || deployments.length === 0) {
      listEl.innerHTML = '<span class="radius-deployments-empty">No applications deployed yet</span>';
      return;
    }

    listEl.innerHTML = deployments.slice(0, 5).map((d) => {
      const timeAgo = formatTimeAgo(d.createdAt);
      const rawName = d.appFile || 'app.bicep';
      const deploymentAppName = rawName.replace(/\.bicep$/, '').replace(/^.*\//, '');
      return `
        <a href="${escapeHTML(d.htmlURL)}" target="_blank" rel="noopener" class="radius-deployment-item">
          <span class="radius-deployment-status">${getStatusIcon(d.status, d.conclusion)}</span>
          <span class="radius-deployment-info">
            <span class="radius-deployment-label">${escapeHTML(deploymentAppName)}</span>
            <span class="radius-deployment-time">${timeAgo}</span>
          </span>
        </a>
      `;
    }).join('');
  } catch {
    const listEl = document.getElementById('radius-deployments-list');
    if (listEl) {
      listEl.innerHTML = '<span class="radius-deployments-empty">Could not load deployments</span>';
    }
  }
}

function getStatusIcon(status: string, conclusion: string): string {
  if (status === 'completed') {
    switch (conclusion) {
      case 'success': return '<span class="radius-status-success">✓</span>';
      case 'failure': return '<span class="radius-status-failure">✗</span>';
      case 'cancelled': return '<span class="radius-status-cancelled">⊘</span>';
      default: return '<span class="radius-status-unknown">?</span>';
    }
  }
  if (status === 'in_progress') return '<span class="radius-status-progress">●</span>';
  return '<span class="radius-status-queued">○</span>';
}

function formatTimeAgo(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  if (diffMins < 1) return 'just now';
  if (diffMins < 60) return `${diffMins}m ago`;
  const diffHours = Math.floor(diffMins / 60);
  if (diffHours < 24) return `${diffHours}h ago`;
  const diffDays = Math.floor(diffHours / 24);
  if (diffDays < 30) return `${diffDays}d ago`;
  return date.toLocaleDateString();
}

function escapeHTML(str: string): string {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

async function checkEnvironmentStatus(btn: HTMLElement, owner: string, repo: string): Promise<void> {
  try {
    if (!chrome?.runtime?.id) return;

    // Check for app file in the repo.
    const hasApp = await chrome.runtime.sendMessage({
      type: 'CHECK_APP',
      owner,
      repo,
    });

    const dropdown = document.getElementById('radius-dropdown');

    if (hasApp?.exists) {
      // App is defined — check if environment is also configured.
      const result = await chrome.runtime.sendMessage({
        type: 'CHECK_ENVIRONMENT',
        owner,
        repo,
      });

      if (result?.configured) {
        // Both app and env exist.
        btn.classList.add('radius-configured');
        btn.dataset.envName = result.name || '';
        btn.title = `Radius environment "${result.name}" configured (${result.provider})`;
        const badge = document.createElement('span');
        badge.className = 'radius-badge';
        badge.textContent = '✓';
        btn.appendChild(badge);

        if (dropdown) {
          dropdown.innerHTML = `
            <button class="radius-dropdown-item radius-dropdown-deploy" data-action="deploy-app">▶ Deploy Application</button>
            <hr class="radius-dropdown-divider">
            <button class="radius-dropdown-item" data-action="define-app">+ Define an Application</button>
            <button class="radius-dropdown-item" data-action="create-env-aws">+ Create AWS environment</button>
            <button class="radius-dropdown-item" data-action="create-env-azure">+ Create Azure environment</button>
          `;
        }
      } else {
        // App defined but no environment — show create environment options.
        if (dropdown) {
          dropdown.innerHTML = `
            <div class="radius-dropdown-hint">You must connect to a cloud platform prior to deploying.</div>
            <button class="radius-dropdown-item" data-action="define-app">+ Define an Application</button>
            <button class="radius-dropdown-item" data-action="create-env-aws">+ Create AWS environment</button>
            <button class="radius-dropdown-item" data-action="create-env-azure">+ Create Azure environment</button>
          `;
        }
      }
    }
    // If no app exists, the default "Define an Application" dropdown stays.
  } catch {
    // Extension context may not be available — that's okay.
  }
}

// Initial injection attempt.
if (!injectRadiusButton()) {
  // If the DOM isn't ready yet, retry a few times.
  let retries = 0;
  const maxRetries = 10;
  const retryInterval = setInterval(() => {
    retries++;
    if (injectRadiusButton() || retries >= maxRetries) {
      clearInterval(retryInterval);
    }
  }, 500);
}

// Inject graph features on initial load.
injectGraphFeatures();

// GitHub uses SPA navigation (turbo/pjax) — re-inject on page changes.
// Listen for turbo:load for GitHub's SPA navigation.
document.addEventListener('turbo:load', () => {
  debouncedInject();
});

// MutationObserver as fallback for cases where Turbo events don't fire.
const observer = new MutationObserver(() => {
  debouncedInject();
});

observer.observe(document.body, { childList: true, subtree: true });
