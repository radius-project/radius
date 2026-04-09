// App creation page logic for the Deploy with Radius extension.
// Standalone page opened from the Deploy button on GitHub when an environment is configured.
//
// SECURITY:
// - No sensitive data is handled on this page.
// - All API calls go through the shared GitHubClient directly to the GitHub API.

import { createClient } from '../shared/api.js';

// --- DOM helpers ---

function $(id: string): HTMLElement {
  const el = document.getElementById(id);
  if (!el) throw new Error(`Element #${id} not found`);
  return el;
}

function show(id: string): void { $(id).classList.remove('hidden'); }
function hide(id: string): void { $(id).classList.add('hidden'); }

function inputVal(id: string): string {
  return ($(id) as HTMLInputElement).value.trim();
}

// --- Page state ---

let currentRepo: { owner: string; repo: string } | null = null;
let envName = '';

// --- Initialization ---

document.addEventListener('DOMContentLoaded', async () => {
  const urlParams = new URLSearchParams(window.location.search);
  const paramOwner = urlParams.get('owner');
  const paramRepo = urlParams.get('repo');
  const paramEnv = urlParams.get('env');

  if (!paramOwner || !paramRepo) {
    show('no-env');
    return;
  }

  currentRepo = { owner: paramOwner, repo: paramRepo };
  $('repo-name').textContent = `${paramOwner}/${paramRepo}`;
  show('repo-context');

  if (paramEnv) {
    envName = paramEnv;
    $('env-name').textContent = `env: ${paramEnv}`;
    show('app-form');
  } else {
    // No environment specified — user needs to set one up first.
    show('no-env');
  }

  // --- Event listeners ---

  $('app-submit').addEventListener('click', handleCreateApp);

  $('retry-btn').addEventListener('click', () => {
    hide('status-section');
    show('app-form');
  });

  $('create-another-btn').addEventListener('click', () => {
    hide('status-section');
    ($('app-name') as HTMLInputElement).value = '';
    show('app-form');
  });
});

// --- Deploy application ---

async function handleCreateApp(): Promise<void> {
  if (!currentRepo) return;
  if (!envName) {
    showError('No environment configured.');
    return;
  }

  const appFile = inputVal('app-name') || 'app.bicep';
  if (!/^[\w./-]+\.bicep$/.test(appFile)) {
    showError('File must be a .bicep file path (e.g., app.bicep).');
    return;
  }

  showLoading('Committing deploy workflow...');

  try {
    const client = await createClient();
    if (!client) { showError('GitHub token not configured.'); return; }

    // Trigger the deploy workflow.
    showLoading('Triggering deployment...');
    await client.triggerDeploy(currentRepo.owner, currentRepo.repo, envName, appFile);

    // Poll for completion.
    showLoading('Deploying application \u2014 this may take a few minutes...');
    await pollAppDeploy(client, currentRepo.owner, currentRepo.repo, envName, appFile);
  } catch (err) {
    showError(`Failed: ${err instanceof Error ? err.message : String(err)}`);
  }
}

async function pollAppDeploy(
  client: Awaited<ReturnType<typeof createClient>>,
  owner: string,
  repo: string,
  env: string,
  appFile: string,
): Promise<void> {
  if (!client) return;

  let attempts = 0;
  const maxAttempts = 30;

  const poll = async (): Promise<void> => {
    attempts++;
    try {
      const status = await client!.getDeployStatus(owner, repo, env);

      if (status.status === 'success') {
        showSuccess(
          'Deployment complete',
          `"${appFile}" has been deployed to environment "${env}".`,
        );
      } else if (status.status === 'failure') {
        showError(
          `Deployment failed. ${status.message}`,
          status.workflowRunURL,
        );
      } else if (attempts < maxAttempts) {
        setTimeout(poll, 5000);
      } else {
        showError(
          'Deployment is taking longer than expected. Check GitHub Actions for status.',
          status.workflowRunURL,
        );
      }
    } catch (err) {
      showError(`Error: ${err instanceof Error ? err.message : String(err)}`);
    }
  };

  setTimeout(poll, 3000);
}

// --- UI helpers ---

function showLoading(message: string): void {
  $('loading-text').textContent = message;
  show('loading');
}

function hideLoading(): void { hide('loading'); }

function showSuccess(title: string, details: string): void {
  hideLoading();
  hide('app-form');

  $('status-section').className = 'status-section status-success';
  $('status-icon').textContent = '';
  $('status-message').textContent = title;
  $('status-details').textContent = details;
  hide('retry-btn');
  show('create-another-btn');
  show('status-actions');
  show('status-section');
}

function showError(message: string, workflowURL?: string): void {
  hideLoading();
  hide('app-form');

  $('status-section').className = 'status-section status-failure';
  $('status-icon').textContent = '';
  $('status-message').textContent = 'Deployment failed';

  let detailsHTML = escapeHTML(message);
  if (workflowURL) {
    detailsHTML += ` <a href="${escapeHTML(workflowURL)}" target="_blank" rel="noopener">View workflow run \u2192</a>`;
  }
  $('status-details').innerHTML = detailsHTML;
  show('retry-btn');
  show('create-another-btn');
  show('status-actions');
  show('status-section');
}

function escapeHTML(str: string): string {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
