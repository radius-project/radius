// Background service worker for the Deploy with Radius extension.
// Handles:
// - Message passing between content script and popup
// - Environment status caching (short-lived, non-sensitive data only)
// - OAuth flow coordination

import { createClient } from '../shared/api.js';

// Cache environment status for 60 seconds to avoid excessive API calls.
const STATUS_CACHE_TTL_MS = 60_000;
const statusCache = new Map<string, { data: unknown; expiry: number }>();

// Open the extension page in a new tab (or focus an existing one).
async function openRadiusTab(owner?: string, repo?: string, step?: string, provider?: string): Promise<void> {
  const pageURL = chrome.runtime.getURL('popup/popup.html');
  const searchParams = new URLSearchParams();
  if (owner) searchParams.set('owner', owner);
  if (repo) searchParams.set('repo', repo);
  if (step) searchParams.set('step', step);
  if (provider) searchParams.set('provider', provider);
  const qs = searchParams.toString();
  const fullURL = qs ? `${pageURL}?${qs}` : pageURL;

  // Check if a Radius tab is already open — reuse it.
  const tabs = await chrome.tabs.query({ url: pageURL + '*' });
  if (tabs.length > 0 && tabs[0].id != null) {
    await chrome.tabs.update(tabs[0].id, { active: true, url: fullURL });
    if (tabs[0].windowId != null) {
      await chrome.windows.update(tabs[0].windowId, { focused: true });
    }
    return;
  }

  await chrome.tabs.create({ url: fullURL });
}

// Open the app-create page in a new tab (or focus an existing one).
async function openAppCreateTab(owner?: string, repo?: string, env?: string): Promise<void> {
  const pageURL = chrome.runtime.getURL('app-create/app-create.html');
  let params = '';
  if (owner && repo) {
    params = `?owner=${encodeURIComponent(owner)}&repo=${encodeURIComponent(repo)}`;
    if (env) {
      params += `&env=${encodeURIComponent(env)}`;
    }
  }
  const fullURL = pageURL + params;

  // Check if an app-create tab is already open — reuse it.
  const tabs = await chrome.tabs.query({ url: pageURL + '*' });
  if (tabs.length > 0 && tabs[0].id != null) {
    await chrome.tabs.update(tabs[0].id, { active: true, url: fullURL });
    if (tabs[0].windowId != null) {
      await chrome.windows.update(tabs[0].windowId, { focused: true });
    }
    return;
  }

  await chrome.tabs.create({ url: fullURL });
}

// When the toolbar icon is clicked, open the page in a new tab.
chrome.action.onClicked.addListener(async (tab) => {
  let owner: string | undefined;
  let repo: string | undefined;
  if (tab.url) {
    const match = tab.url.match(/^https:\/\/github\.com\/([^/]+)\/([^/]+)/);
    if (match) {
      owner = match[1];
      repo = match[2];
    }
  }
  await openRadiusTab(owner, repo);
});

// Listen for messages from content scripts and popup.
chrome.runtime.onMessage.addListener(
   (message: { type: string; owner?: string; repo?: string; name?: string; env?: string; step?: string; provider?: string },
   _sender: chrome.runtime.MessageSender,
   sendResponse: (response?: unknown) => void) => {
    switch (message.type) {
      case 'OPEN_TAB':
        // Open the extension page in a new tab.
        openRadiusTab(message.owner, message.repo, message.step, message.provider)
          .then(() => sendResponse({ ok: true }))
          .catch(() => sendResponse({ ok: false }));
        return true; // async response

      case 'OPEN_APP_TAB':
        // Open the app-create page in a new tab.
        openAppCreateTab(message.owner, message.repo, message.env)
          .then(() => sendResponse({ ok: true }))
          .catch(() => sendResponse({ ok: false }));
        return true; // async response

      case 'CHECK_ENVIRONMENT':
        handleCheckEnvironment(message.owner!, message.repo!)
          .then(sendResponse)
          .catch(() => sendResponse(null));
        return true; // async response

      case 'CHECK_APP':
        handleCheckApp(message.owner!, message.repo!)
          .then(sendResponse)
          .catch(() => sendResponse(null));
        return true; // async response

      case 'LIST_DEPLOYMENTS':
        handleListDeployments(message.owner!, message.repo!)
          .then(sendResponse)
          .catch(() => sendResponse([]));
        return true; // async response

      default:
        sendResponse(null);
    }
  },
);

async function handleCheckEnvironment(
  owner: string,
  repo: string,
): Promise<{ configured: boolean; name?: string; provider?: string } | null> {
  const cacheKey = `${owner}/${repo}`;

  // Check cache first.
  const cached = statusCache.get(cacheKey);
  if (cached && Date.now() < cached.expiry) {
    return cached.data as { configured: boolean; name?: string; provider?: string };
  }

  try {
    const client = await createClient();
    if (!client) return null;

    // Check for a default "dev" environment.
    const env = await client.getEnvironmentStatus(owner, repo, 'dev');
    const result = env
      ? { configured: true, name: env.name, provider: env.provider }
      : { configured: false };

    // Cache the result (non-sensitive — just name + provider + boolean).
    statusCache.set(cacheKey, { data: result, expiry: Date.now() + STATUS_CACHE_TTL_MS });

    return result;
  } catch {
    return null;
  }
}

async function handleCheckApp(
  owner: string,
  repo: string,
): Promise<{ exists: boolean; filename?: string } | null> {
  const cacheKey = `app:${owner}/${repo}`;

  const cached = statusCache.get(cacheKey);
  if (cached && Date.now() < cached.expiry) {
    return cached.data as { exists: boolean; filename?: string };
  }

  try {
    const client = await createClient();
    if (!client) return null;

    const result = await client.checkAppFile(owner, repo);
    const data = { exists: result.exists, filename: result.filename };
    statusCache.set(cacheKey, { data, expiry: Date.now() + STATUS_CACHE_TTL_MS });
    return data;
  } catch {
    return null;
  }
}

async function handleListDeployments(
  owner: string,
  repo: string,
): Promise<unknown[]> {
  const cacheKey = `deployments:${owner}/${repo}`;

  const cached = statusCache.get(cacheKey);
  if (cached && Date.now() < cached.expiry) {
    return cached.data as unknown[];
  }

  try {
    const client = await createClient();
    if (!client) return [];

    const deployments = await client.listDeployments(owner, repo);
    statusCache.set(cacheKey, { data: deployments, expiry: Date.now() + STATUS_CACHE_TTL_MS });
    return deployments;
  } catch {
    return [];
  }
}

// Clear cache on storage changes (e.g., backend URL changed).
chrome.storage.onChanged.addListener(() => {
  statusCache.clear();
});
