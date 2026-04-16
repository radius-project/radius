// Popup logic for the Deploy with Radius extension.
// Two-step wizard: 1. Define Application  2. Create Environment (credentials)
//
// SECURITY:
// - Sensitive inputs (client secrets, role ARNs) are never persisted in extension storage.
// - Client secrets are transmitted over HTTPS only and sent directly to the backend.
// - The extension never logs or caches sensitive credential values.
// - Input fields use autocomplete="off" / autocomplete="new-password" to prevent browser caching.

import { AWS_REGIONS } from '../shared/types.js';
import type { CloudProvider } from '../shared/types.js';
import { createClient, getGitHubToken, setGitHubToken } from '../shared/api.js';
import { startDeviceFlow, getClientID, setClientID, getAppSlug, setAppSlug } from '../shared/device-flow.js';

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

// --- Wizard state ---

let currentProvider: CloudProvider = 'aws';
let currentRepo: { owner: string; repo: string } | null = null;
let wizardEnvName = '';
let wizardAppFile = '';

function setWizardStep(step: 1 | 2 | 3): void {
  document.querySelectorAll('.wizard-step').forEach((el) => {
    const s = Number((el as HTMLElement).dataset.step);
    el.classList.toggle('active', s === step);
    el.classList.toggle('completed', s < step);
  });
}

function hideAllSections(): void {
  hide('app-form');
  hide('aws-form');
  hide('azure-form');
  hide('provider-selector');
  hide('verify-section');
  hide('status-section');
  hide('deploy-form');
  hide('deps-form');
  hide('deps-form');
}

// --- Initialization ---

document.addEventListener('DOMContentLoaded', async () => {
  // Detect the current repo — from URL params (tab mode) or active tab.
  try {
    const urlParams = new URLSearchParams(window.location.search);
    const paramOwner = urlParams.get('owner');
    const paramRepo = urlParams.get('repo');

    if (paramOwner && paramRepo) {
      currentRepo = { owner: paramOwner, repo: paramRepo };
      $('repo-name').textContent = `${paramOwner}/${paramRepo}`;
      show('repo-context');
    } else {
      const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
      if (tab?.url) {
        const match = tab.url.match(/^https:\/\/github\.com\/([^/]+)\/([^/]+)/);
        if (match) {
          currentRepo = { owner: match[1], repo: match[2] };
          $('repo-name').textContent = `${match[1]}/${match[2]}`;
          show('repo-context');
        }
      }
    }
  } catch {
    // Not on a GitHub page — that's fine.
  }

  // Check if GitHub token is configured.
  const githubToken = await getGitHubToken();
  if (!githubToken) {
    show('setup-required');

    const clientId = await getClientID();
    const appSlug = await getAppSlug();

    // If Client ID is already saved, skip to install + sign-in steps.
    if (clientId) {
      hide('setup-client-id');
      show('setup-steps');
      const installLink = $('install-app-link') as HTMLAnchorElement;
      installLink.href = `https://github.com/apps/${appSlug || 'radius-deploy'}/installations/new`;
    }

    // Save Client ID + slug, then show install/sign-in steps.
    $('setup-client-id-save').addEventListener('click', async () => {
      const newClientId = ($('setup-client-id-input') as HTMLInputElement).value.trim();
      const newSlug = ($('setup-app-slug') as HTMLInputElement).value.trim();
      if (!newClientId || !newSlug) return;
      await setClientID(newClientId);
      await setAppSlug(newSlug);
      hide('setup-client-id');
      show('setup-steps');
      const installLink = $('install-app-link') as HTMLAnchorElement;
      installLink.href = `https://github.com/apps/${newSlug}/installations/new`;
    });

    $('github-sign-in').addEventListener('click', () => {
      hide('setup-required');
      show('device-flow');
      startDeviceFlow({
        onUserCode: (userCode, verificationUri) => {
          $('device-code').textContent = userCode;
          const link = $('device-link') as HTMLAnchorElement;
          link.href = verificationUri;
        },
        onPolling: () => {
          $('device-status').textContent = 'Waiting for authorization...';
        },
        onSuccess: async (token) => {
          await setGitHubToken(token);
          hide('device-flow');
          const urlParams = new URLSearchParams(window.location.search);
          if (urlParams.get('step') === 'copilot') {
            handleCopilotStep();
          } else {
            showStep1();
          }
        },
        onError: (error) => {
          $('device-status').textContent = `Error: ${error}`;
        },
      });
    });
    $('show-settings').addEventListener('click', () => {
      hide('setup-required');
      show('settings-bar');
      // Pre-populate saved values.
      if (clientId) {
        ($('client-id') as HTMLInputElement).value = clientId;
      }
    });
  } else {
    // Check if a specific step was requested via URL params.
    const urlParams = new URLSearchParams(window.location.search);
    const requestedStep = urlParams.get('step');
    const requestedProvider = urlParams.get('provider');

    if (requestedStep === 'env') {
      // Go directly to environment creation.
      if (requestedProvider === 'azure') {
        currentProvider = 'azure';
      }
      showStep2();
    } else if (requestedStep === 'deploy') {
      showDeployStep();
    } else if (requestedStep === 'copilot') {
      // Commit workflows then open Copilot.
      handleCopilotStep();
    } else {
      showStep1();
    }
  }

  // Populate AWS regions dropdown.
  const regionSelect = $('aws-region') as HTMLSelectElement;
  for (const region of AWS_REGIONS) {
    const opt = document.createElement('option');
    opt.value = region;
    opt.textContent = region;
    if (region === 'us-east-1') opt.selected = true;
    regionSelect.appendChild(opt);
  }

  // --- Event listeners ---

  // Settings save.
  $('save-settings').addEventListener('click', async () => {
    const clientId = inputVal('client-id');
    if (clientId) {
      await setClientID(clientId);
    }
    const token = inputVal('api-key');
    if (token) {
      await setGitHubToken(token);
    }
    if (!clientId && !token) return;
    hide('settings-bar');

    // If we came from the copilot flow, resume it after saving settings.
    const urlParams = new URLSearchParams(window.location.search);
    if (urlParams.get('step') === 'copilot') {
      handleCopilotStep();
    } else {
      showStep1();
    }
  });

  // Settings toggle.
  $('settings-toggle')?.addEventListener('click', () => {
    const bar = $('settings-bar');
    bar.classList.toggle('hidden');
  });

  // Reset button — clears all stored credentials and reloads.
  $('reset-btn')?.addEventListener('click', async () => {
    await chrome.storage.local.remove(['radius_github_token', 'radius_client_id', 'radius_app_slug']);
    window.location.reload();
  });

  // Provider tabs.
  document.querySelectorAll('.tab').forEach((tab) => {
    tab.addEventListener('click', () => {
      const provider = (tab as HTMLElement).dataset.provider as CloudProvider;
      switchProvider(provider);
    });
  });

  // Azure auth type toggle — show/hide client secret field.
  document.querySelectorAll('input[name="azure-auth-type"]').forEach((radio) => {
    radio.addEventListener('change', () => {
      const value = (radio as HTMLInputElement).value;
      if (value === 'ServicePrincipal') {
        show('azure-sp-fields');
      } else {
        hide('azure-sp-fields');
        (document.getElementById('azure-client-secret') as HTMLInputElement).value = '';
      }
    });
  });

  // Step 1: Define application — create the Bicep file in the repo.
  $('app-next').addEventListener('click', async () => {
    if (!currentRepo) {
      showFinalError('Navigate to a GitHub repository first.');
      return;
    }

    const appFile = inputVal('app-bicep-file');
    if (!appFile) {
      showFinalError('Bicep file name is required.');
      return;
    }

    showLoading('Defining application...');

    try {
      const client = await createClient();
      if (!client) { showFinalError('GitHub token not configured.'); return; }

      const result = await client.createAppFile(currentRepo.owner, currentRepo.repo, appFile);

      wizardAppFile = result.filename;
      const msg = result.created
        ? `Application "${result.filename}" has been defined and committed to the repository.`
        : `Application "${result.filename}" already exists in the repository.`;
      hideLoading();
      hideAllSections();
      show('wizard-steps');
      setWizardStep(1);

      $('status-section').className = 'status-section status-success';
      $('status-icon').textContent = '';
      $('status-message').textContent = 'Application defined';
      $('status-details').innerHTML = `${msg}<br><br>`;

      // Add action buttons.
      const btnContainer = document.createElement('div');
      btnContainer.style.display = 'flex';
      btnContainer.style.gap = '8px';
      btnContainer.style.justifyContent = 'center';
      btnContainer.style.marginTop = '8px';

      const envBtn = document.createElement('button');
      envBtn.className = 'btn btn-primary';
      envBtn.textContent = 'Create Environment';
      envBtn.addEventListener('click', () => showStep2());

      const anotherBtn = document.createElement('button');
      anotherBtn.className = 'btn btn-outline';
      anotherBtn.textContent = 'Create Another Application';
      anotherBtn.addEventListener('click', () => showStep1());

      btnContainer.appendChild(envBtn);
      btnContainer.appendChild(anotherBtn);
      $('status-details').appendChild(btnContainer);
      show('status-section');
    } catch (err) {
      showFinalError(`Failed to create app file: ${err instanceof Error ? err.message : String(err)}`);
    }
  });

  // Step 2: Verify credentials.
  $('aws-submit').addEventListener('click', handleAWSVerify);
  $('azure-submit').addEventListener('click', handleAzureVerify);

  // Dependencies form: save and continue to deploy.
  $('deps-submit').addEventListener('click', async () => {
    if (!currentRepo || !wizardEnvName) return;
    showLoading('Saving dependencies...');
    try {
      const client = await createClient();
      if (!client) { showVerifyError('GitHub token not configured.'); return; }

      await client.saveDependencies(currentRepo.owner, currentRepo.repo, wizardEnvName, {
        namespace: inputVal('deps-k8s-namespace'),
        appImage: inputVal('deps-app-image'),
        vpcId: inputVal('deps-vpc-id'),
        subnetIds: inputVal('deps-subnet-ids'),
        dbPassword: inputVal('deps-db-password'),
      });

      hideLoading();
      // Show success on a clean page.
      hideAllSections();
      hide('wizard-steps');
      $('page-title').textContent = 'Setup with Radius';
      $('status-section').className = 'status-section status-success';
      $('status-icon').textContent = '';
      $('status-message').textContent = 'Environment configured';
      $('status-details').innerHTML = '';
      const msg = document.createElement('p');
      msg.textContent = `Environment "${wizardEnvName}" is ready with credentials verified and dependencies saved.`;
      $('status-details').appendChild(msg);
      const btnRow = document.createElement('div');
      btnRow.style.cssText = 'display:flex; gap:8px; justify-content:center; margin-top:16px;';
      const deployBtn = document.createElement('button');
      deployBtn.className = 'btn btn-primary';
      deployBtn.textContent = 'Deploy Application';
      deployBtn.addEventListener('click', () => showDeployForm());
      const anotherBtn = document.createElement('button');
      anotherBtn.className = 'btn btn-outline';
      anotherBtn.textContent = 'Create Another Environment';
      anotherBtn.addEventListener('click', () => showStep2());
      btnRow.appendChild(deployBtn);
      btnRow.appendChild(anotherBtn);
      $('status-details').appendChild(btnRow);
      show('status-section');
    } catch (err) {
      showVerifyError(`Failed: ${err instanceof Error ? err.message : String(err)}`);
    }
  });

  // Dependencies form: skip to success.
  $('deps-skip').addEventListener('click', () => {
    hideAllSections();
    hide('wizard-steps');
    $('page-title').textContent = 'Setup with Radius';
    $('status-section').className = 'status-section status-success';
    $('status-icon').textContent = '';
    $('status-message').textContent = 'Environment configured';
    $('status-details').innerHTML = '';
    const msg2 = document.createElement('p');
    msg2.textContent = `Environment "${wizardEnvName}" is ready with credentials verified.`;
    $('status-details').appendChild(msg2);
    const btnRow2 = document.createElement('div');
    btnRow2.style.cssText = 'display:flex; gap:8px; justify-content:center; margin-top:16px;';
    const deployBtn2 = document.createElement('button');
    deployBtn2.className = 'btn btn-primary';
    deployBtn2.textContent = 'Deploy Application';
    deployBtn2.addEventListener('click', () => showDeployForm());
    const anotherBtn2 = document.createElement('button');
    anotherBtn2.className = 'btn btn-outline';
    anotherBtn2.textContent = 'Create Another Environment';
    anotherBtn2.addEventListener('click', () => showStep2());
    btnRow2.appendChild(deployBtn2);
    btnRow2.appendChild(anotherBtn2);
    $('status-details').appendChild(btnRow2);
    show('status-section');
  });

  // Deploy: trigger application deployment.
  $('deploy-submit').addEventListener('click', handleDeploy);

  // Verification retry — go back to step 2 (credentials).
  $('verify-retry').addEventListener('click', () => {
    showStep2();
  });

  });

// --- Step navigation ---

const COPILOT_SKILL_URL =
  'https://raw.githubusercontent.com/radius-project/radius/main/.github/skills/app-modeling/SKILL.md';

async function handleCopilotStep(): Promise<void> {
  if (!currentRepo) {
    showStep1();
    return;
  }
  showLoading('Committing workflows and opening Copilot...');
  try {
    const client = await createClient();
    if (!client) {
      hideLoading();
      showStep1();
      return;
    }
    await client.commitAllWorkflows(currentRepo.owner, currentRepo.repo);
    const prompt = `Create an application definition.\n\nRead ${COPILOT_SKILL_URL}`;
    const copilotUrl = `https://github.com/copilot?repo=${encodeURIComponent(currentRepo.owner + '/' + currentRepo.repo)}&prompt=${encodeURIComponent(prompt)}`;
    window.open(copilotUrl, '_blank');
    hideLoading();
    hideAllSections();
    $('page-title').textContent = 'Setup with Radius';
    $('status-section').className = 'status-section status-success';
    $('status-icon').textContent = '';
    $('status-message').textContent = 'Copilot opened';
    $('status-details').textContent = 'Workflows committed and Copilot opened in a new tab. Define your application there, then come back to create an environment.';
    show('status-section');
  } catch (err) {
    hideLoading();
    showVerifyError(`Failed: ${err instanceof Error ? err.message : String(err)}`);
  }
}

function showStep1(): void {
  hideAllSections();
  show('wizard-steps');
  setWizardStep(1);
  show('app-form');
  $('page-title').textContent = 'Setup with Radius';
}

function showStep2(): void {
  hideAllSections();
  show('wizard-steps');
  show('provider-selector');
  setWizardStep(2);
  $('page-title').textContent = 'Setup with Radius';
  if (currentProvider === 'aws') {
    show('aws-form');
  } else {
    show('azure-form');
  }
}

function showDeployStep(): void {
  hideAllSections();
  hide('wizard-steps');
  show('deploy-form');
  $('page-title').textContent = 'Deploy with Radius';

  // Pre-populate the deploy form with detected values.
  if (currentRepo) {
    (async () => {
      try {
        const client = await createClient();
        if (!client) return;

        // Check for the app file to pre-populate the filename.
        const appResult = await client.checkAppFile(currentRepo!.owner, currentRepo!.repo);
        if (appResult.exists) {
          ($('deploy-app-file') as HTMLInputElement).value = appResult.filename;
        }

        // Check for the environment to pre-populate the name.
        const envResult = await client.getEnvironmentStatus(currentRepo!.owner, currentRepo!.repo, 'dev');
        if (envResult) {
          ($('deploy-env-name') as HTMLInputElement).value = envResult.name;
        }
      } catch {
        // Best-effort pre-population.
      }
    })();
  }
}

// Show deploy form with pre-populated values from the wizard flow.
function showDeployForm(): void {
  hideAllSections();
  hide('wizard-steps');
  show('deploy-form');
  $('page-title').textContent = 'Deploy with Radius';
  if (wizardAppFile) {
    ($('deploy-app-file') as HTMLInputElement).value = wizardAppFile;
    // Derive app name from filename.
    const name = wizardAppFile.replace(/\.bicep$/, '').replace(/.*\//, '');
    ($('deploy-app-name') as HTMLInputElement).value = name;
  }
  if (wizardEnvName) {
    ($('deploy-env-name') as HTMLInputElement).value = wizardEnvName;
  }
}

// --- Provider switching ---

function switchProvider(provider: CloudProvider): void {
  currentProvider = provider;
  document.querySelectorAll('.tab').forEach((tab) => {
    tab.classList.toggle('active', (tab as HTMLElement).dataset.provider === provider);
  });
  hide('aws-form');
  hide('azure-form');
  if (provider === 'aws') {
    show('aws-form');
  } else {
    show('azure-form');
  }
}

// --- Step 1: Verify credentials ---

async function handleAWSVerify(): Promise<void> {
  if (!currentRepo) {
    showVerifyError('Navigate to a GitHub repository to create an environment.');
    return;
  }

  const name = inputVal('aws-env-name');
  const roleARN = inputVal('aws-role-arn');
  const region = inputVal('aws-region');
  const k8sCluster = inputVal('aws-k8s-cluster');

  if (!name) { showVerifyError('Environment name is required.'); return; }
  if (!roleARN) { showVerifyError('IAM Role ARN is required.'); return; }
  if (!validateARN(roleARN)) { showVerifyError('Invalid IAM Role ARN format. Expected: arn:aws:iam::ACCOUNT:role/NAME'); return; }
  if (!region) { showVerifyError('AWS Region is required.'); return; }
  if (!k8sCluster) { showVerifyError('EKS cluster name is required.'); return; }

  // Extract account ID from the ARN (arn:aws:iam::ACCOUNT_ID:role/...).
  const accountID = extractAccountIDFromARN(roleARN);
  if (!accountID) { showVerifyError('Could not extract AWS Account ID from IAM Role ARN.'); return; }

  showLoading('Setting up credentials and confirming authentication...');

  try {
    const client = await createClient();
    if (!client) { showVerifyError('GitHub token not configured.'); return; }

    // Create environment + set credential variables (needed for verification).
    const result = await client.createAWSEnvironment(currentRepo.owner, currentRepo.repo, {
      name,
      roleARN,
      region,
      accountID,
    });

    // Save the cluster name as a GitHub Environment variable.
    if (k8sCluster) {
      await client.setVariable(currentRepo.owner, currentRepo.repo, name, 'RADIUS_K8S_CLUSTER', k8sCluster);
    }

    wizardEnvName = result.name;

    // Trigger verification and poll.
    await verifyAndPoll(client, currentRepo.owner, currentRepo.repo, result.name);
  } catch (err) {
    showVerifyError(`Failed: ${err instanceof Error ? err.message : String(err)}`);
  }
}

async function handleAzureVerify(): Promise<void> {
  if (!currentRepo) {
    showVerifyError('Navigate to a GitHub repository to create an environment.');
    return;
  }

  const name = inputVal('azure-env-name');
  const tenantID = inputVal('azure-tenant-id');
  const clientID = inputVal('azure-client-id');
  const subscriptionID = inputVal('azure-subscription-id');
  const resourceGroup = inputVal('azure-resource-group');
  const authType = (document.querySelector('input[name="azure-auth-type"]:checked') as HTMLInputElement)?.value || 'WorkloadIdentity';
  const clientSecret = inputVal('azure-client-secret');

  if (!name) { showVerifyError('Environment name is required.'); return; }
  if (!tenantID) { showVerifyError('Tenant ID is required.'); return; }
  if (!validateUUID(tenantID)) { showVerifyError('Tenant ID must be a valid UUID.'); return; }
  if (!clientID) { showVerifyError('Client ID is required.'); return; }
  if (!validateUUID(clientID)) { showVerifyError('Client ID must be a valid UUID.'); return; }
  if (!subscriptionID) { showVerifyError('Subscription ID is required.'); return; }
  if (!validateUUID(subscriptionID)) { showVerifyError('Subscription ID must be a valid UUID.'); return; }
  if (authType === 'ServicePrincipal' && !clientSecret) {
    showVerifyError('Client Secret is required for Service Principal authentication.');
    return;
  }

  showLoading('Setting up credentials and confirming authentication...');

  try {
    const client = await createClient();
    if (!client) { showVerifyError('GitHub token not configured.'); return; }

    const result = await client.createAzureEnvironment(currentRepo.owner, currentRepo.repo, {
      name,
      tenantID,
      clientID,
      subscriptionID,
      resourceGroup: resourceGroup || undefined,
      authType: authType as 'WorkloadIdentity' | 'ServicePrincipal',
      clientSecret: authType === 'ServicePrincipal' ? clientSecret : undefined,
    });

    // Clear client secret from DOM immediately.
    (document.getElementById('azure-client-secret') as HTMLInputElement).value = '';

    wizardEnvName = result.name;

    // Trigger verification and poll.
    await verifyAndPoll(client, currentRepo.owner, currentRepo.repo, result.name);

    // Pre-populate resource group if provided.
    if (resourceGroup) {
      ($('deps-resource-group') as HTMLInputElement).value = resourceGroup;
    }
  } catch (err) {
    (document.getElementById('azure-client-secret') as HTMLInputElement).value = '';
    showVerifyError(`Failed: ${err instanceof Error ? err.message : String(err)}`);
  }
}

async function verifyAndPoll(client: Awaited<ReturnType<typeof createClient>>, owner: string, repo: string, envName: string): Promise<void> {
  if (!client) return;

  showLoading('Triggering credential verification...');

  try {
    await client.triggerVerification(owner, repo, envName);
  } catch (err) {
    showVerifyError(`Failed to trigger verification: ${err instanceof Error ? err.message : String(err)}`);
    return;
  }

  showLoading('Verifying credentials — this may take a minute...');

  let attempts = 0;
  const maxAttempts = 30;

  const poll = async (): Promise<void> => {
    attempts++;
    try {
      const status = await client!.getVerificationStatus(owner, repo, envName);

      if (status.status === 'success') {
        hideLoading();
        // Verification passed — show dependencies form.
        hideAllSections();
        show('wizard-steps');
        show('deps-form');
      } else if (status.status === 'failure') {
        showVerifyError(
          `Credential verification failed. ${status.message}`,
          status.workflowRunURL,
        );
      } else if (attempts < maxAttempts) {
        showLoading('Verifying credentials — this may take a minute...', status.workflowRunURL);
        setTimeout(poll, 5000);
      } else {
        showVerifyError(
          'Verification is taking longer than expected. Check GitHub Actions for status.',
          status.workflowRunURL,
        );
      }
    } catch (err) {
      showVerifyError(`Verification error: ${err instanceof Error ? err.message : String(err)}`);
    }
  };

  // Give the workflow a moment to start.
  setTimeout(poll, 3000);
}

async function handleDeploy(): Promise<void> {
  if (!currentRepo) {
    showFinalError('Navigate to a GitHub repository first.');
    return;
  }

  const appFile = inputVal('deploy-app-file');
  const appName = inputVal('deploy-app-name');
  const envName = inputVal('deploy-env-name');

  if (!appFile) { showFinalError('Application file is required.'); return; }
  if (!envName) { showFinalError('Environment name is required.'); return; }

  showLoading('Preparing deploy workflow...');

  try {
    const client = await createClient();
    if (!client) { showFinalError('GitHub token not configured.'); return; }

    showLoading('Triggering deployment...');

    // Trigger the deploy workflow with the app file and name.
    await client.triggerDeploy(currentRepo.owner, currentRepo.repo, envName, appFile, appName);

    showLoading('Deployment triggered — waiting for status...');

    // Poll for deploy status.
    let attempts = 0;
    const maxAttempts = 60; // Up to 5 minutes at 5s intervals.

    const poll = async (): Promise<void> => {
      attempts++;
      try {
        const status = await client!.getDeployStatus(
          currentRepo!.owner,
          currentRepo!.repo,
          envName,
        );

        if (status.status === 'success') {
          hideLoading();
          hideAllSections();
          hide('wizard-steps');

          $('status-section').className = 'status-section status-success';
          $('status-icon').textContent = '';
          $('status-message').textContent = 'Deployment successful';
          const detailsText = `Application "${appFile}" has been deployed using environment "${envName}".`;
          $('status-details').textContent = detailsText;
          if (status.workflowRunURL) {
            $('status-details').innerHTML = escapeHTML(detailsText) +
              ` <a href="${escapeHTML(status.workflowRunURL)}" target="_blank" rel="noopener">View run \u2192</a>`;
          }
          show('status-section');
        } else if (status.status === 'failure') {
          hideLoading();
          hideAllSections();
          hide('wizard-steps');

          $('status-section').className = 'status-section status-failure';
          $('status-icon').textContent = '';
          $('status-message').textContent = 'Deployment failed';
          let details = status.message || `Deployment of "${appFile}" failed.`;
          if (status.workflowRunURL) {
            details += ` <a href="${escapeHTML(status.workflowRunURL)}" target="_blank" rel="noopener">View run \u2192</a>`;
          }
          $('status-details').innerHTML = details;
          show('status-section');
        } else if (attempts < maxAttempts) {
          const msg = status.status === 'in_progress'
            ? 'Deployment in progress...'
            : 'Waiting for deployment to start...';
          showLoading(msg, status.workflowRunURL);
          setTimeout(poll, 5000);
        } else {
          hideLoading();
          hideAllSections();
          hide('wizard-steps');

          $('status-section').className = 'status-section status-failure';
          $('status-icon').textContent = '';
          $('status-message').textContent = 'Deployment timed out';
          let details = 'The deployment is taking longer than expected. Check GitHub Actions for status.';
          if (status.workflowRunURL) {
            details += ` <a href="${escapeHTML(status.workflowRunURL)}" target="_blank" rel="noopener">View run \u2192</a>`;
          }
          $('status-details').innerHTML = details;
          show('status-section');
        }
      } catch (err) {
        showFinalError(`Deploy status error: ${err instanceof Error ? err.message : String(err)}`);
      }
    };

    // Give the workflow a moment to start.
    setTimeout(poll, 5000);
  } catch (err) {
    showFinalError(`Failed to deploy: ${err instanceof Error ? err.message : String(err)}`);
  }
}



// --- UI state helpers ---

function showLoading(message: string, workflowURL?: string): void {
  $('loading-text').textContent = message;
  const link = $('loading-link') as HTMLAnchorElement;
  if (workflowURL) {
    link.href = workflowURL;
    link.classList.remove('hidden');
  } else {
    link.removeAttribute('href');
    link.classList.add('hidden');
  }
  show('loading');
}

function hideLoading(): void { hide('loading'); }

function showVerifyError(message: string, workflowURL?: string): void {
  hideLoading();
  hideAllSections();
  show('wizard-steps');

  $('verify-section').className = 'status-section status-failure';
  $('verify-icon').textContent = '';
  $('verify-message').textContent = 'Authentication failed';

  let detailsHTML = escapeHTML(message);
  if (workflowURL) {
    detailsHTML += ` <a href="${escapeHTML(workflowURL)}" target="_blank" rel="noopener">View workflow run →</a>`;
  }
  $('verify-details').innerHTML = detailsHTML;
  show('verify-actions');
  show('verify-section');
}

function showFinalError(message: string): void {
  hideLoading();
  hideAllSections();
  show('wizard-steps');

  $('status-section').className = 'status-section status-failure';
  $('status-icon').textContent = '';
  $('status-message').textContent = 'Error';
  $('status-details').textContent = message;
  show('status-section');
}

// --- Validation ---

function validateARN(arn: string): boolean {
  return /^arn:aws:iam::\d{12}:role\/[\w+=,.@-]+$/.test(arn);
}

function extractAccountIDFromARN(arn: string): string | null {
  const match = arn.match(/^arn:aws:iam::(\d{12}):role\//);
  return match ? match[1] : null;
}

function validateUUID(uuid: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(uuid);
}

function escapeHTML(str: string): string {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}
