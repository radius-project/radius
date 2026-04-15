// GitHub API client that replaces the Go backend.
// All GitHub API calls are made directly from the extension using the user's OAuth token.
// Sensitive fields (client secrets) are encrypted via NaCl sealed box before storage.

import type {
  CreateAWSEnvironmentRequest,
  CreateAzureEnvironmentRequest,
  EnvironmentResponse,
  VerificationResponse,
  DeploymentSummary,
  CreateAppFileResponse,
} from './types.js';

const GITHUB_API = 'https://api.github.com';

export class GitHubClient {
  constructor(private token: string) {}

  // --- Repository info ---

  private async getRepoID(owner: string, repo: string): Promise<number> {
    const data = await this.get<{ id: number; default_branch: string }>(
      `/repos/${owner}/${repo}`,
    );
    return data.id;
  }

  private async getDefaultBranch(owner: string, repo: string): Promise<string> {
    const data = await this.get<{ default_branch: string }>(
      `/repos/${owner}/${repo}`,
    );
    return data.default_branch;
  }

  // --- App installation check ---

  async isAppInstalledOnRepo(owner: string, repo: string): Promise<boolean> {
    try {
      await this.get(`/repos/${owner}/${repo}/installation`);
      return true;
    } catch {
      return false;
    }
  }

  // --- Environment CRUD ---

  async createEnvironment(owner: string, repo: string, envName: string): Promise<void> {
    await this.request('PUT', `/repos/${owner}/${repo}/environments/${envName}`);
  }

  async environmentExists(owner: string, repo: string, envName: string): Promise<boolean> {
    try {
      await this.get(`/repos/${owner}/${repo}/environments/${envName}`);
      return true;
    } catch (err) {
      if (err instanceof GitHubAPIError && err.status === 404) return false;
      throw err;
    }
  }

  async deleteEnvironment(owner: string, repo: string, envName: string): Promise<void> {
    await this.request('DELETE', `/repos/${owner}/${repo}/environments/${envName}`);
  }

  // --- Environment Variables ---

  async setVariable(
    owner: string, repo: string, envName: string,
    key: string, value: string,
  ): Promise<void> {
    const repoID = await this.getRepoID(owner, repo);
    const payload = JSON.stringify({ name: key, value });

    // Try create first.
    const createResp = await this.rawRequest(
      'POST',
      `/repositories/${repoID}/environments/${envName}/variables`,
      payload,
    );

    if (createResp.status === 201) return;

    // 409 = already exists, update instead.
    if (createResp.status === 409) {
      await this.request(
        'PATCH',
        `/repositories/${repoID}/environments/${envName}/variables/${key}`,
        payload,
      );
      return;
    }

    throw new GitHubAPIError(createResp.status, `Failed to set variable ${key}`);
  }

  async getVariables(
    owner: string, repo: string, envName: string,
  ): Promise<Record<string, string>> {
    const repoID = await this.getRepoID(owner, repo);
    const data = await this.get<{
      variables: Array<{ name: string; value: string }>;
    }>(`/repositories/${repoID}/environments/${envName}/variables`);

    const vars: Record<string, string> = {};
    for (const v of data.variables) {
      vars[v.name] = v.value;
    }
    return vars;
  }

  // --- Environment Secrets (NaCl sealed box encryption) ---

  async setSecret(
    owner: string, repo: string, envName: string,
    key: string, value: string,
  ): Promise<void> {
    const repoID = await this.getRepoID(owner, repo);

    // Get the environment's public key.
    const pk = await this.get<{ key_id: string; key: string }>(
      `/repositories/${repoID}/environments/${envName}/secrets/public-key`,
    );

    const encrypted = await encryptSecret(pk.key, value);

    await this.request(
      'PUT',
      `/repositories/${repoID}/environments/${envName}/secrets/${key}`,
      JSON.stringify({ encrypted_value: encrypted, key_id: pk.key_id }),
    );
  }

  // --- High-level: Create AWS Environment ---

  async createAWSEnvironment(
    owner: string, repo: string, config: CreateAWSEnvironmentRequest,
  ): Promise<EnvironmentResponse> {
    await this.createEnvironment(owner, repo, config.name);

    const vars: Record<string, string> = {
      AWS_IAM_ROLE_ARN: config.roleARN,
      AWS_REGION: config.region,
    };
    if (config.accountID) {
      vars['AWS_ACCOUNT_ID'] = config.accountID;
    }

    const variablesSet: string[] = [];
    for (const [key, value] of Object.entries(vars)) {
      await this.setVariable(owner, repo, config.name, key, value);
      variablesSet.push(key);
    }

    return {
      name: config.name,
      provider: 'aws',
      githubEnvironmentCreated: true,
      variablesSet,
      credentialsVerified: false,
    };
  }

  // --- High-level: Create Azure Environment ---

  async createAzureEnvironment(
    owner: string, repo: string, config: CreateAzureEnvironmentRequest,
  ): Promise<EnvironmentResponse> {
    await this.createEnvironment(owner, repo, config.name);

    const vars: Record<string, string> = {
      AZURE_TENANT_ID: config.tenantID,
      AZURE_CLIENT_ID: config.clientID,
      AZURE_SUBSCRIPTION_ID: config.subscriptionID,
    };
    if (config.resourceGroup) {
      vars['AZURE_RESOURCE_GROUP'] = config.resourceGroup;
    }

    const variablesSet: string[] = [];
    for (const [key, value] of Object.entries(vars)) {
      await this.setVariable(owner, repo, config.name, key, value);
      variablesSet.push(key);
    }

    // Store client secret for Service Principal auth.
    if (config.authType === 'ServicePrincipal' && config.clientSecret) {
      await this.setSecret(owner, repo, config.name, 'AZURE_CLIENT_SECRET', config.clientSecret);
    }

    return {
      name: config.name,
      provider: 'azure',
      githubEnvironmentCreated: true,
      variablesSet,
      credentialsVerified: false,
    };
  }

  // --- High-level: Get Environment Status ---

  async getEnvironmentStatus(
    owner: string, repo: string, envName: string,
  ): Promise<EnvironmentResponse | null> {
    const exists = await this.environmentExists(owner, repo, envName);
    if (!exists) return null;

    const vars = await this.getVariables(owner, repo, envName);

    let provider = '';
    if (vars['AWS_IAM_ROLE_ARN']) provider = 'aws';
    else if (vars['AZURE_TENANT_ID']) provider = 'azure';

    return {
      name: envName,
      provider,
      githubEnvironmentCreated: true,
      variablesSet: Object.keys(vars),
      credentialsVerified: false,
    };
  }

  // --- Dependencies ---

  async saveDependencies(
    owner: string, repo: string, envName: string,
    deps: { cluster?: string; namespace?: string; appImage?: string; vpcId?: string; subnetIds?: string; dbPassword?: string },
  ): Promise<string[]> {
    const vars: Record<string, string> = {};
    if (deps.cluster) vars['RADIUS_K8S_CLUSTER'] = deps.cluster;
    if (deps.namespace) vars['RADIUS_K8S_NAMESPACE'] = deps.namespace;
    if (deps.appImage) vars['RADIUS_APP_IMAGE'] = deps.appImage;
    if (deps.vpcId) vars['RADIUS_VPC_ID'] = deps.vpcId;
    if (deps.subnetIds) {
      // Convert comma-separated input to JSON array format: ["subnet-a","subnet-b"]
      const ids = deps.subnetIds.split(',').map((s) => s.trim()).filter(Boolean);
      vars['RADIUS_SUBNET_IDS'] = JSON.stringify(ids);
    }

    const set: string[] = [];
    for (const [key, value] of Object.entries(vars)) {
      await this.setVariable(owner, repo, envName, key, value);
      set.push(key);
    }

    // Store password as a secret, not a variable.
    if (deps.dbPassword) {
      await this.setSecret(owner, repo, envName, 'RADIUS_DB_PASSWORD', deps.dbPassword);
      set.push('RADIUS_DB_PASSWORD');
    }

    return set;
  }

  // --- File operations (commit workflows, app file) ---

  private async fileExists(owner: string, repo: string, path: string, branch: string): Promise<boolean> {
    try {
      await this.get(`/repos/${owner}/${repo}/contents/${path}?ref=${branch}`);
      return true;
    } catch {
      return false;
    }
  }

  private async getFileContent(owner: string, repo: string, path: string, branch: string): Promise<string | null> {
    try {
      const data = await this.get<{ content: string; encoding: string }>(
        `/repos/${owner}/${repo}/contents/${path}?ref=${branch}`,
      );
      if (data.encoding === 'base64') {
        // GitHub returns base64 with line wrapping — strip newlines before decoding.
        const raw = data.content.replace(/[\r\n]/g, '');
        const bytes = Uint8Array.from(atob(raw), (c) => c.charCodeAt(0));
        return new TextDecoder().decode(bytes);
      }
      return data.content;
    } catch {
      return null;
    }
  }

  private async commitFile(
    owner: string, repo: string, path: string,
    message: string, content: string, branch: string,
  ): Promise<void> {
    // Check if file exists to get its SHA (required for updates).
    let sha: string | undefined;
    try {
      const existing = await this.get<{ sha: string }>(
        `/repos/${owner}/${repo}/contents/${path}?ref=${branch}`,
      );
      sha = existing.sha;
    } catch {
      // File doesn't exist yet.
    }

    const payload: Record<string, string> = {
      message,
      content: utf8ToBase64(content),
      branch,
    };
    if (sha) {
      payload['sha'] = sha;
    }

    await this.request(
      'PUT',
      `/repos/${owner}/${repo}/contents/${path}`,
      JSON.stringify(payload),
    );
  }

  async commitAllWorkflows(owner: string, repo: string): Promise<void> {
    const branch = await this.getDefaultBranch(owner, repo);

    const verifyContent = await this.getFileContent(
      owner, repo, '.github/workflows/radius-verify-credentials.yml', branch,
    );
    const deployContent = await this.getFileContent(
      owner, repo, '.github/workflows/radius-deploy.yml', branch,
    );

    // Only commit workflows that are missing or have changed.
    if (verifyContent !== VERIFY_WORKFLOW) {
      await this.commitFile(
        owner, repo,
        '.github/workflows/radius-verify-credentials.yml',
        verifyContent === null
          ? 'radius: add verification and deploy workflows'
          : 'radius: update verification workflow',
        VERIFY_WORKFLOW, branch,
      );
    }

    if (deployContent !== DEPLOY_WORKFLOW) {
      await this.commitFile(
        owner, repo,
        '.github/workflows/radius-deploy.yml',
        deployContent === null
          ? 'radius: add verification and deploy workflows'
          : 'radius: update deploy workflow',
        DEPLOY_WORKFLOW, branch,
      );
    }
  }

  // --- Application file ---

  async createAppFile(
    owner: string, repo: string, filename = 'app.bicep',
  ): Promise<CreateAppFileResponse> {
    const branch = await this.getDefaultBranch(owner, repo);
    const exists = await this.fileExists(owner, repo, filename, branch);
    if (exists) {
      return { filename, created: false };
    }

    const appName = filename.replace(/\.bicep$/, '').replace(/.*\//, '');
    const content = `import radius as radius\n\n@description('The Radius application')\nresource app 'Applications.Core/applications@2023-10-01-preview' = {\n  name: '${appName}'\n  properties: {\n    environment: radius.envVar('RADIUS_ENVIRONMENT_ID')\n  }\n}\n`;

    await this.commitFile(owner, repo, filename, `Add Radius application: ${filename}`, content, branch);

    // Commit workflows alongside the app file.
    await this.commitAllWorkflows(owner, repo);

    return { filename, created: true };
  }

  async checkAppFile(
    owner: string, repo: string, filename = 'app.bicep',
  ): Promise<{ filename: string; exists: boolean }> {
    const branch = await this.getDefaultBranch(owner, repo);
    const exists = await this.fileExists(owner, repo, filename, branch);
    return { filename, exists };
  }

  // --- Verification ---

  async triggerVerification(
    owner: string, repo: string, envName: string,
  ): Promise<VerificationResponse> {
    const branch = await this.getDefaultBranch(owner, repo);

    // Only commit the workflow if it's missing or has changed.
    const existing = await this.getFileContent(
      owner, repo, '.github/workflows/radius-verify-credentials.yml', branch,
    );
    if (existing !== VERIFY_WORKFLOW) {
      await this.commitFile(
        owner, repo,
        '.github/workflows/radius-verify-credentials.yml',
        existing === null
          ? 'radius: add verification workflow'
          : 'radius: update verification workflow',
        VERIFY_WORKFLOW, branch,
      );
      // Wait for GitHub to index the workflow.
      await delay(3000);
    }

    // Trigger the workflow from the default branch.
    const payload = JSON.stringify({
      ref: branch,
      inputs: { environment: envName },
    });

    // Retry — GitHub may need a moment to index the workflow.
    let lastErr: Error | null = null;
    for (let i = 0; i < 10; i++) {
      try {
        await this.request(
          'POST',
          `/repos/${owner}/${repo}/actions/workflows/radius-verify-credentials.yml/dispatches`,
          payload,
        );
        return {
          provider: '',
          status: 'pending',
          message: 'Verification workflow triggered. Poll the status endpoint for results.',
        };
      } catch (err) {
        lastErr = err as Error;
        await delay(3000 + i * 2000);
      }
    }
    throw lastErr;
  }

  async getVerificationStatus(
    owner: string, repo: string, _envName: string,
  ): Promise<VerificationResponse> {
    const data = await this.get<{
      workflow_runs: Array<{
        status: string;
        conclusion: string;
        html_url: string;
      }>;
    }>(`/repos/${owner}/${repo}/actions/workflows/radius-verify-credentials.yml/runs?per_page=1`);

    if (data.workflow_runs.length === 0) {
      return { provider: '', status: 'pending', message: 'No verification runs found.' };
    }

    const run = data.workflow_runs[0];
    if (run.status === 'completed' && run.conclusion === 'success') {
      return { provider: '', status: 'success', message: 'Cloud credentials verified successfully.', workflowRunURL: run.html_url };
    }
    if (run.status === 'completed') {
      return { provider: '', status: 'failure', message: `Verification failed: ${run.conclusion}`, workflowRunURL: run.html_url };
    }
    return { provider: '', status: 'in_progress', message: `Verification is ${run.status}.`, workflowRunURL: run.html_url };
  }

  // --- Deploy ---

  async triggerDeploy(
    owner: string, repo: string, envName: string, appFile = 'app.bicep', appName = '',
  ): Promise<void> {
    const branch = await this.getDefaultBranch(owner, repo);

    // Only commit the workflow if it's missing or has changed.
    const existing = await this.getFileContent(
      owner, repo, '.github/workflows/radius-deploy.yml', branch,
    );
    if (existing !== DEPLOY_WORKFLOW) {
      await this.commitFile(
        owner, repo,
        '.github/workflows/radius-deploy.yml',
        existing === null
          ? 'radius: add deploy workflow'
          : 'radius: update deploy workflow',
        DEPLOY_WORKFLOW, branch,
      );
      await delay(3000);
    }

    // Derive app name from filename if not provided.
    if (!appName) {
      appName = appFile.replace(/\.bicep$/, '').replace(/.*\//, '');
    }

    // Retry — GitHub may need a moment to index the workflow.
    let lastErr: Error | null = null;
    for (let i = 0; i < 10; i++) {
      try {
        await this.request(
          'POST',
          `/repos/${owner}/${repo}/actions/workflows/radius-deploy.yml/dispatches`,
          JSON.stringify({
            ref: branch,
            inputs: { environment: envName, app_file: appFile, app_name: appName },
          }),
        );
        return;
      } catch (err) {
        lastErr = err as Error;
        await delay(3000 + i * 2000);
      }
    }
    throw lastErr;
  }

  async getDeployStatus(
    owner: string, repo: string, _envName: string,
  ): Promise<VerificationResponse> {
    const data = await this.get<{
      workflow_runs: Array<{
        status: string;
        conclusion: string;
        html_url: string;
      }>;
    }>(`/repos/${owner}/${repo}/actions/workflows/radius-deploy.yml/runs?per_page=1`);

    if (data.workflow_runs.length === 0) {
      return { provider: '', status: 'pending', message: 'No deploy runs found.' };
    }

    const run = data.workflow_runs[0];
    if (run.status === 'completed' && run.conclusion === 'success') {
      return { provider: '', status: 'success', message: 'Deployment complete.', workflowRunURL: run.html_url };
    }
    if (run.status === 'completed') {
      return { provider: '', status: 'failure', message: `Deployment failed: ${run.conclusion}`, workflowRunURL: run.html_url };
    }
    return { provider: '', status: 'in_progress', message: `Deployment is ${run.status}.`, workflowRunURL: run.html_url };
  }

  async listDeployments(
    owner: string, repo: string,
  ): Promise<DeploymentSummary[]> {
    const data = await this.get<{
      workflow_runs: Array<{
        id: number;
        status: string;
        conclusion: string;
        html_url: string;
        created_at: string;
        head_branch: string;
      }>;
    }>(`/repos/${owner}/${repo}/actions/workflows/radius-deploy.yml/runs?per_page=5`);

    return data.workflow_runs.map((run) => ({
      id: run.id,
      status: run.status,
      conclusion: run.conclusion,
      htmlURL: run.html_url,
      createdAt: run.created_at,
      headBranch: run.head_branch,
    }));
  }

  private async get<T>(path: string): Promise<T> {
    const resp = await this.rawRequest('GET', path);
    if (resp.status === 401) {
      await clearExpiredToken();
      throw new GitHubAPIError(401, 'GitHub token expired. Please sign in again.');
    }
    if (!resp.ok) throw new GitHubAPIError(resp.status, await resp.text());
    return resp.json();
  }

  private async request(method: string, path: string, body?: string): Promise<void> {
    const resp = await this.rawRequest(method, path, body);
    if (resp.status === 401) {
      await clearExpiredToken();
      throw new GitHubAPIError(401, 'GitHub token expired. Please sign in again.');
    }
    if (resp.status >= 300 && resp.status !== 409) {
      throw new GitHubAPIError(resp.status, await resp.text());
    }
  }

  private async rawRequest(method: string, path: string, body?: string): Promise<Response> {
    const headers: Record<string, string> = {
      'Authorization': `Bearer ${this.token}`,
      'Accept': 'application/vnd.github+json',
      'X-GitHub-Api-Version': '2022-11-28',
    };
    if (body) {
      headers['Content-Type'] = 'application/json';
    }
    return fetch(`${GITHUB_API}${path}`, { method, headers, body });
  }
}

export class GitHubAPIError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
    this.name = 'GitHubAPIError';
  }
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// Clear an expired token from storage so the sign-in flow appears on next load.
async function clearExpiredToken(): Promise<void> {
  await chrome.storage.local.remove('radius_github_token');
}

// Encode a UTF-8 string to base64, handling characters outside the Latin1 range.
function utf8ToBase64(str: string): string {
  const bytes = new TextEncoder().encode(str);
  let binary = '';
  for (const b of bytes) {
    binary += String.fromCharCode(b);
  }
  return btoa(binary);
}

// --- NaCl sealed box encryption for GitHub Secrets API ---

async function encryptSecret(publicKeyB64: string, secret: string): Promise<string> {
  // Pure JS sealed box: tweetnacl (box encryption) + blakejs (BLAKE2b nonce).
  // This matches libsodium's crypto_box_seal without requiring WebAssembly.
  const nacl = await import('tweetnacl');
  const blake = await import('blakejs');

  const publicKeyBytes = Uint8Array.from(atob(publicKeyB64), (c) => c.charCodeAt(0));
  const messageBytes = new TextEncoder().encode(secret);

  // Generate ephemeral keypair.
  const ephemeralKeys = nacl.default.box.keyPair();

  // Derive nonce using BLAKE2b(ephemeral_pk || recipient_pk, 24 bytes).
  // This matches libsodium's crypto_box_seal nonce derivation.
  const nonceInput = new Uint8Array(64);
  nonceInput.set(ephemeralKeys.publicKey, 0);
  nonceInput.set(publicKeyBytes, 32);
  const nonce = blake.default.blake2b(nonceInput, undefined, 24);

  const encrypted = nacl.default.box(messageBytes, nonce, publicKeyBytes, ephemeralKeys.secretKey);

  // Sealed box format: ephemeral_pk (32) + ciphertext (with MAC).
  const sealed = new Uint8Array(32 + encrypted.length);
  sealed.set(ephemeralKeys.publicKey, 0);
  sealed.set(encrypted, 32);

  let binary = '';
  for (const b of sealed) {
    binary += String.fromCharCode(b);
  }
  return btoa(binary);
}

// --- Static workflow templates ---
// These are the same workflows generated by the Go backend, but as static strings.
// They are provider-agnostic — both Azure and AWS steps are included, gated by
// GitHub Actions `if:` conditions.

const VERIFY_WORKFLOW = `# This workflow is auto-generated by Radius to verify cloud credentials.
name: Radius - Verify Cloud Credentials

on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'GitHub Environment name'
        required: true
        default: 'dev'

permissions:
  id-token: write
  contents: read

jobs:
  verify:
    name: Verify cloud credentials
    runs-on: ubuntu-latest
    environment: \${{ inputs.environment }}
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Azure Login (OIDC)
        if: \${{ vars.AZURE_CLIENT_ID != '' }}
        uses: azure/login@v2
        with:
          client-id: \${{ vars.AZURE_CLIENT_ID }}
          tenant-id: \${{ vars.AZURE_TENANT_ID }}
          subscription-id: \${{ vars.AZURE_SUBSCRIPTION_ID }}

      - name: Verify Azure access
        if: \${{ vars.AZURE_CLIENT_ID != '' }}
        run: |
          set -euo pipefail
          echo "Verifying Azure access..."
          az account show --query '{name:name, state:state}' --output table
          echo ""
          echo "✅ Azure credentials are working correctly."

      - name: Configure AWS Credentials (OIDC)
        if: \${{ vars.AWS_IAM_ROLE_ARN != '' }}
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: \${{ vars.AWS_IAM_ROLE_ARN }}
          aws-region: \${{ vars.AWS_REGION }}

      - name: Verify AWS access
        if: \${{ vars.AWS_IAM_ROLE_ARN != '' }}
        run: |
          set -euo pipefail
          echo "Verifying AWS access..."
          CALLER_ARN=$(aws sts get-caller-identity --query 'Arn' --output text)
          echo "\${CALLER_ARN}" | sed 's/[0-9]\\{12\\}/****/g'
          echo ""
          echo "✅ AWS credentials are working correctly."

      - name: Setup EKS cluster access
        if: \${{ vars.AWS_IAM_ROLE_ARN != '' && vars.RADIUS_K8S_CLUSTER != '' }}
        run: |
          CLUSTER="\${{ vars.RADIUS_K8S_CLUSTER }}"
          REGION="\${{ vars.AWS_REGION }}"
          ROLE_ARN="\${{ vars.AWS_IAM_ROLE_ARN }}"

          echo "Checking EKS auth mode..."
          AUTH_MODE=$(aws eks describe-cluster --name "$CLUSTER" --region "$REGION" --query 'cluster.accessConfig.authenticationMode' --output text 2>/dev/null || echo "UNKNOWN")
          echo "Auth mode: $AUTH_MODE"

          if [ "$AUTH_MODE" = "CONFIG_MAP" ]; then
            echo "Cluster uses ConfigMap auth. Updating to API_AND_CONFIG_MAP..."
            aws eks update-cluster-config --name "$CLUSTER" --region "$REGION" --access-config authenticationMode=API_AND_CONFIG_MAP || echo "Could not update auth mode"
            sleep 10
          fi

          echo "Creating EKS access entry for $ROLE_ARN..."
          aws eks create-access-entry --cluster-name "$CLUSTER" --principal-arn "$ROLE_ARN" --type STANDARD --region "$REGION" 2>&1 || echo "Access entry may already exist"
          aws eks associate-access-policy --cluster-name "$CLUSTER" --principal-arn "$ROLE_ARN" --policy-arn arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy --access-scope type=cluster --region "$REGION" 2>&1 || echo "Access policy may already be associated"

          echo "Testing EKS cluster connectivity..."
          ENDPOINT=$(aws eks describe-cluster --name "$CLUSTER" --region "$REGION" --query 'cluster.endpoint' --output text)
          CA_DATA=$(aws eks describe-cluster --name "$CLUSTER" --region "$REGION" --query 'cluster.certificateAuthority.data' --output text)
          TOKEN=$(aws eks get-token --cluster-name "$CLUSTER" --region "$REGION" --output json | jq -r '.status.token')
          printf 'apiVersion: v1\\nclusters:\\n- cluster:\\n    certificate-authority-data: %s\\n    server: %s\\n  name: eks\\ncontexts:\\n- context:\\n    cluster: eks\\n    user: eks-user\\n  name: eks\\ncurrent-context: eks\\nkind: Config\\nusers:\\n- name: eks-user\\n  user:\\n    token: %s\\n' "$CA_DATA" "$ENDPOINT" "$TOKEN" > /tmp/test-eks
          for i in $(seq 1 12); do
            if kubectl --kubeconfig /tmp/test-eks get nodes 2>&1; then
              echo "✅ EKS access configured."
              rm -f /tmp/test-eks
              exit 0
            fi
            echo "Waiting for access policy to propagate... ($i/12)"
            sleep 10
          done
          echo "❌ EKS cluster connectivity failed after retries."
          echo "Role ARN: $ROLE_ARN"
          aws sts get-caller-identity || true
          rm -f /tmp/test-eks
          exit 1
`;

const DEPLOY_WORKFLOW = `# This workflow is auto-generated by Radius to deploy applications.
# It creates an ephemeral k3d cluster for the Radius control plane, connects
# to the user's target cluster (EKS/AKS), and runs rad deploy.
name: Radius - Deploy Application

on:
  push:
    branches: [main]
    paths:
      - 'app.bicep'
      - 'bicepconfig.json'
      - '*.bicep'
      - '.github/workflows/radius-deploy.yml'
  workflow_dispatch:
    inputs:
      environment:
        description: 'GitHub Environment name'
        required: true
        default: 'dev'
      app_file:
        description: 'Bicep application file to deploy'
        required: true
        default: 'app.bicep'
      app_name:
        description: 'Application name'
        required: false
        default: 'app'
      image:
        description: 'Container image for the application (e.g. ghcr.io/sk593/demo:latest)'
        required: false
        default: ''

permissions:
  id-token: write
  contents: read
  packages: write

env:
  ENVIRONMENT: \${{ inputs.environment || 'dev' }}
  APP_FILE: \${{ inputs.app_file || 'app.bicep' }}
  APP_NAME: \${{ inputs.app_name || 'app' }}
  APP_IMAGE: \${{ inputs.image || vars.RADIUS_APP_IMAGE || '' }}
  RESOURCE_TYPES_CONTRIB_REPO: https://github.com/radius-project/resource-types-contrib.git
  RESOURCE_TYPES_CONTRIB_REF: main

jobs:
  deploy:
    name: Deploy with Radius
    runs-on: ubuntu-latest
    environment: \${{ inputs.environment || 'dev' }}
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Azure Login (OIDC)
        if: \${{ vars.AZURE_CLIENT_ID != '' }}
        uses: azure/login@v2
        with:
          client-id: \${{ vars.AZURE_CLIENT_ID }}
          tenant-id: \${{ vars.AZURE_TENANT_ID }}
          subscription-id: \${{ vars.AZURE_SUBSCRIPTION_ID }}

      - name: Configure AWS Credentials (OIDC)
        if: \${{ vars.AWS_IAM_ROLE_ARN != '' }}
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: \${{ vars.AWS_IAM_ROLE_ARN }}
          aws-region: \${{ vars.AWS_REGION }}

      - name: Get target cluster kubeconfig
        run: mkdir -p "$HOME/.kube"

      - name: Connect to AKS cluster
        if: \${{ vars.AZURE_CLIENT_ID != '' && vars.RADIUS_K8S_CLUSTER != '' }}
        run: |
          az aks get-credentials \\
            --resource-group "\${{ vars.AZURE_RESOURCE_GROUP }}" \\
            --name "\${{ vars.RADIUS_K8S_CLUSTER }}" \\
            --subscription "\${{ vars.AZURE_SUBSCRIPTION_ID }}" \\
            --file "$HOME/.kube/target-cluster"

      - name: Connect to EKS cluster
        if: \${{ vars.AWS_IAM_ROLE_ARN != '' && vars.RADIUS_K8S_CLUSTER != '' }}
        run: |
          CLUSTER="\${{ vars.RADIUS_K8S_CLUSTER }}"
          REGION="\${{ vars.AWS_REGION }}"
          ROLE_ARN="\${{ vars.AWS_IAM_ROLE_ARN }}"
          TARGET="$HOME/.kube/target-cluster"

          # Ensure the IAM role has access to the EKS cluster.
          # Creates an access entry if one doesn't already exist.
          echo "Ensuring EKS access entry for $ROLE_ARN..."
          aws eks create-access-entry \\
            --cluster-name "$CLUSTER" \\
            --principal-arn "$ROLE_ARN" \\
            --type STANDARD \\
            --region "$REGION" 2>/dev/null || echo "Access entry already exists"
          aws eks associate-access-policy \\
            --cluster-name "$CLUSTER" \\
            --principal-arn "$ROLE_ARN" \\
            --policy-arn arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy \\
            --access-scope type=cluster \\
            --region "$REGION" 2>/dev/null || echo "Access policy already associated"

          # Build a static kubeconfig with a bearer token instead of exec-based auth.
          # The exec-based config requires aws CLI inside the container, which Radius
          # images don't have.
          ENDPOINT=$(aws eks describe-cluster --name "$CLUSTER" --region "$REGION" --query 'cluster.endpoint' --output text)
          CA_DATA=$(aws eks describe-cluster --name "$CLUSTER" --region "$REGION" --query 'cluster.certificateAuthority.data' --output text)
          TOKEN=$(aws eks get-token --cluster-name "$CLUSTER" --region "$REGION" --output json | jq -r '.status.token')
          printf 'apiVersion: v1\\nclusters:\\n- cluster:\\n    certificate-authority-data: %s\\n    server: %s\\n  name: eks\\ncontexts:\\n- context:\\n    cluster: eks\\n    user: eks-user\\n  name: eks\\ncurrent-context: eks\\nkind: Config\\nusers:\\n- name: eks-user\\n  user:\\n    token: %s\\n' "$CA_DATA" "$ENDPOINT" "$TOKEN" > "$TARGET"
          echo "EKS kubeconfig saved with static token"
          kubectl --kubeconfig "$TARGET" cluster-info || echo "WARNING: Could not connect to EKS cluster"

      - name: Install k3d
        run: curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

      - name: Create ephemeral Radius control plane cluster
        run: |
          k3d cluster create radius-cp \
            --volume /var/run/docker.sock:/var/run/docker.sock \
            --volume "\${{ github.workspace }}/:/app/demo" \
            --wait
          kubectl wait --for=condition=Ready node --all --timeout=120s

      - name: Install Radius CLI
        run: |
          wget -q "https://raw.githubusercontent.com/radius-project/radius/main/deploy/install.sh" -O - | /bin/bash
          rad version

      - name: Install Terraform
        uses: hashicorp/setup-terraform@v3
        with:
          terraform_wrapper: false

      - name: Install Radius on control plane
        run: |
          rad install kubernetes \\
            --set rp.publicEndpointOverride=localhost \\
            --set global.imageRegistry=ghcr.io/sk593 \\
            --set global.imageTag=latest \\
            --set de.image=ghcr.io/radius-project/deployment-engine \\
            --set de.tag=latest \\
            --set dashboard.image=ghcr.io/radius-project/dashboard \\
            --set dashboard.tag=latest
          kubectl wait --for=condition=Available deployment --all -n radius-system --timeout=300s

      - name: Patch dynamic-rp with Docker support
        run: |
          # Mount Docker socket, source directory, install docker-cli via init container,
          # and run as root to access the socket
          kubectl patch deployment dynamic-rp -n radius-system --type=json -p='[
            {"op": "add", "path": "/spec/template/spec/volumes/-",
             "value": {"name": "docker-sock", "hostPath": {"path": "/var/run/docker.sock", "type": "Socket"}}},
            {"op": "add", "path": "/spec/template/spec/volumes/-",
             "value": {"name": "docker-bin", "emptyDir": {}}},
            {"op": "add", "path": "/spec/template/spec/volumes/-",
             "value": {"name": "app-source", "hostPath": {"path": "/app/demo", "type": "Directory"}}},
            {"op": "add", "path": "/spec/template/spec/containers/0/volumeMounts/-",
             "value": {"name": "docker-sock", "mountPath": "/var/run/docker.sock"}},
            {"op": "add", "path": "/spec/template/spec/containers/0/volumeMounts/-",
             "value": {"name": "docker-bin", "mountPath": "/usr/local/bin/docker", "subPath": "docker"}},
            {"op": "add", "path": "/spec/template/spec/containers/0/volumeMounts/-",
             "value": {"name": "app-source", "mountPath": "/app/demo", "readOnly": true}},
            {"op": "replace", "path": "/spec/template/spec/containers/0/securityContext",
             "value": {"allowPrivilegeEscalation": false, "runAsUser": 0}},
            {"op": "add", "path": "/spec/template/spec/containers/0/resources",
             "value": {"requests": {"memory": "512Mi", "cpu": "500m"}, "limits": {"memory": "2Gi", "cpu": "2"}}},
            {"op": "add", "path": "/spec/template/spec/initContainers/-",
             "value": {
               "name": "install-docker-cli",
               "image": "docker:cli",
               "command": ["cp", "/usr/local/bin/docker", "/out/docker"],
               "volumeMounts": [{"name": "docker-bin", "mountPath": "/out"}]
             }}
          ]'
          kubectl rollout status deployment/dynamic-rp -n radius-system --timeout=120s

          # Wait for old pods to terminate, then get the running pod
          sleep 10
          kubectl delete pod -n radius-system -l app.kubernetes.io/name=dynamic-rp --field-selector=status.phase!=Running --ignore-not-found || true
          kubectl wait --for=condition=Ready pod -n radius-system -l app.kubernetes.io/name=dynamic-rp --timeout=120s

          # Verify docker and source are accessible
          POD=$(kubectl get pods -n radius-system -l app.kubernetes.io/name=dynamic-rp --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}')
          kubectl exec -n radius-system "$POD" -c dynamic-rp -- docker version
          kubectl exec -n radius-system "$POD" -c dynamic-rp -- ls /app/demo/Dockerfile

          # Create a Docker config secret for GHCR authentication so Terraform's Docker provider can push images.
          DOCKER_CONFIG_JSON=$(echo -n '{"auths":{"ghcr.io":{"auth":"'$(echo -n "token:\${{ secrets.GITHUB_TOKEN }}" | base64 -w0)'"}}}')
          kubectl create secret generic docker-config -n radius-system --from-literal=config.json="$DOCKER_CONFIG_JSON" --dry-run=client -o yaml | kubectl apply -f -

          # Mount a writable emptyDir at /root/.docker and use an init container to copy
          # the config.json from the secret. This way buildx can write to /root/.docker/buildx.
          kubectl patch deployment dynamic-rp -n radius-system --type=json -p='[
            {"op": "add", "path": "/spec/template/spec/volumes/-",
             "value": {"name": "docker-config-secret", "secret": {"secretName": "docker-config"}}},
            {"op": "add", "path": "/spec/template/spec/volumes/-",
             "value": {"name": "docker-config", "emptyDir": {}}},
            {"op": "add", "path": "/spec/template/spec/containers/0/volumeMounts/-",
             "value": {"name": "docker-config", "mountPath": "/root/.docker"}},
            {"op": "add", "path": "/spec/template/spec/containers/0/env/-",
             "value": {"name": "DOCKER_CONFIG", "value": "/root/.docker"}},
            {"op": "add", "path": "/spec/template/spec/initContainers/-",
             "value": {
               "name": "copy-docker-config",
               "image": "busybox:latest",
               "command": ["sh", "-c", "cp /secret/config.json /docker-config/config.json"],
               "volumeMounts": [
                 {"name": "docker-config-secret", "mountPath": "/secret", "readOnly": true},
                 {"name": "docker-config", "mountPath": "/docker-config"}
               ]
             }}
          ]'
          kubectl rollout status deployment/dynamic-rp -n radius-system --timeout=120s
          sleep 10
          kubectl wait --for=condition=Ready pod -n radius-system -l app.kubernetes.io/name=dynamic-rp --timeout=120s

          # Verify docker auth is available
          POD=$(kubectl get pods -n radius-system -l app.kubernetes.io/name=dynamic-rp --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}')
          kubectl exec -n radius-system "$POD" -c dynamic-rp -- cat /root/.docker/config.json

      - name: Configure external deployment target
        run: |
          TARGET_KUBECONFIG="$HOME/.kube/target-cluster"

          if [ ! -f "$TARGET_KUBECONFIG" ]; then
            echo "No target kubeconfig found, resources will deploy to k3d cluster"
            exit 0
          fi

          # Refresh EKS token right before creating the secret.
          if [ -n "\${{ vars.AWS_IAM_ROLE_ARN }}" ] && [ -n "\${{ vars.RADIUS_K8S_CLUSTER }}" ]; then
            echo "Generating fresh EKS token..."
            CLUSTER="\${{ vars.RADIUS_K8S_CLUSTER }}"
            REGION="\${{ vars.AWS_REGION }}"
            ENDPOINT=$(aws eks describe-cluster --name "$CLUSTER" --region "$REGION" --query 'cluster.endpoint' --output text)
            CA_DATA=$(aws eks describe-cluster --name "$CLUSTER" --region "$REGION" --query 'cluster.certificateAuthority.data' --output text)
            TOKEN=$(aws eks get-token --cluster-name "$CLUSTER" --region "$REGION" --output json | jq -r '.status.token')
            printf 'apiVersion: v1\\nclusters:\\n- cluster:\\n    certificate-authority-data: %s\\n    server: %s\\n  name: eks\\ncontexts:\\n- context:\\n    cluster: eks\\n    user: eks-user\\n  name: eks\\ncurrent-context: eks\\nkind: Config\\nusers:\\n- name: eks-user\\n  user:\\n    token: %s\\n' "$CA_DATA" "$ENDPOINT" "$TOKEN" > "$TARGET_KUBECONFIG"
          fi

          echo "Configuring Radius to deploy to external target cluster..."
          kubectl delete secret target-kubeconfig -n radius-system --ignore-not-found
          kubectl create secret generic target-kubeconfig --namespace radius-system --from-file=config="$TARGET_KUBECONFIG"

          # Check if deployments already have the volume mount (from a previous run).
          if ! kubectl get deployment applications-rp -n radius-system -o jsonpath='{.spec.template.spec.volumes[*].name}' | grep -q target-kubeconfig; then
            PATCH='[
              {"op":"add","path":"/spec/template/spec/volumes/-","value":{"name":"target-kubeconfig","secret":{"secretName":"target-kubeconfig"}}},
              {"op":"add","path":"/spec/template/spec/containers/0/volumeMounts/-","value":{"name":"target-kubeconfig","mountPath":"/etc/radius/target-kubeconfig","readOnly":true}},
              {"op":"add","path":"/spec/template/spec/containers/0/env/-","value":{"name":"RADIUS_TARGET_KUBECONFIG","value":"/etc/radius/target-kubeconfig/config"}}
            ]'
            for deploy in applications-rp dynamic-rp; do
              kubectl patch deployment $deploy -n radius-system --type=json -p="$PATCH"
            done

            # Set KUBE_CONFIG_PATH on dynamic-rp so Terraform's kubernetes provider deploys to the target cluster
            kubectl patch deployment dynamic-rp -n radius-system --type=json -p='[
              {"op":"add","path":"/spec/template/spec/containers/0/env/-","value":{"name":"KUBE_CONFIG_PATH","value":"/etc/radius/target-kubeconfig/config"}}
            ]'
          else
            # Secret updated — restart pods to pick up new token.
            kubectl rollout restart deployment/applications-rp -n radius-system
            kubectl rollout restart deployment/dynamic-rp -n radius-system
          fi

          # Inject AWS session credentials into Radius pods so Terraform's AWS provider
          # can create AWS resources. These come from the runner's OIDC exchange.
          if [ -n "$AWS_ACCESS_KEY_ID" ]; then
            echo "Injecting AWS session credentials into Radius pods..."
            for deploy in applications-rp dynamic-rp; do
              kubectl patch deployment $deploy -n radius-system --type=json -p="[
                {\\"op\\":\\"add\\",\\"path\\":\\"/spec/template/spec/containers/0/env/-\\",\\"value\\":{\\"name\\":\\"AWS_ACCESS_KEY_ID\\",\\"value\\":\\"$AWS_ACCESS_KEY_ID\\"}},
                {\\"op\\":\\"add\\",\\"path\\":\\"/spec/template/spec/containers/0/env/-\\",\\"value\\":{\\"name\\":\\"AWS_SECRET_ACCESS_KEY\\",\\"value\\":\\"$AWS_SECRET_ACCESS_KEY\\"}},
                {\\"op\\":\\"add\\",\\"path\\":\\"/spec/template/spec/containers/0/env/-\\",\\"value\\":{\\"name\\":\\"AWS_SESSION_TOKEN\\",\\"value\\":\\"$AWS_SESSION_TOKEN\\"}},
                {\\"op\\":\\"add\\",\\"path\\":\\"/spec/template/spec/containers/0/env/-\\",\\"value\\":{\\"name\\":\\"AWS_REGION\\",\\"value\\":\\"\${{ vars.AWS_REGION }}\\"}}
              ]"
            done
          fi

          echo "Waiting for rollouts..."
          kubectl rollout status deployment/applications-rp -n radius-system --timeout=300s
          kubectl rollout status deployment/dynamic-rp -n radius-system --timeout=300s
          echo "External deployment target configured."

      - name: Configure Radius environment
        run: |
          NAMESPACE="\${{ vars.RADIUS_K8S_NAMESPACE || 'default' }}"

          # Ensure namespace exists on target cluster before Radius deploys into it.
          TARGET_KUBECONFIG="$HOME/.kube/target-cluster"
          if [ -f "$TARGET_KUBECONFIG" ]; then
            echo "Ensuring namespace $NAMESPACE exists on target cluster..."
            kubectl --kubeconfig "$TARGET_KUBECONFIG" get namespace "$NAMESPACE" 2>/dev/null || \
              kubectl --kubeconfig "$TARGET_KUBECONFIG" create namespace "$NAMESPACE"
          fi

          rad workspace create kubernetes default
          rad group create default
          rad group switch default
          rad env create "$ENVIRONMENT" --namespace "$NAMESPACE"
          rad env switch "$ENVIRONMENT"

      - name: Register Azure credentials with Radius
        if: \${{ vars.AZURE_CLIENT_ID != '' }}
        run: |
          rad credential register azure wi \\
            --client-id "\${{ vars.AZURE_CLIENT_ID }}" \\
            --tenant-id "\${{ vars.AZURE_TENANT_ID }}"
          rad env update "$ENVIRONMENT" \\
            --azure-subscription-id "\${{ vars.AZURE_SUBSCRIPTION_ID }}" \\
            --azure-resource-group "\${{ vars.AZURE_RESOURCE_GROUP }}"

      - name: Register AWS credentials with Radius
        if: \${{ vars.AWS_IAM_ROLE_ARN != '' }}
        run: |
          rad credential register aws access-key \
            --access-key-id "$AWS_ACCESS_KEY_ID" \
            --secret-access-key "$AWS_SECRET_ACCESS_KEY"
          rad env update "$ENVIRONMENT" \
            --aws-region "\${{ vars.AWS_REGION }}" \
            --aws-account-id "\${{ vars.AWS_ACCOUNT_ID }}"

      - name: Register resource types from resource-types-contrib
        run: |
          REPO_RAW="https://raw.githubusercontent.com/radius-project/resource-types-contrib/\${{ env.RESOURCE_TYPES_CONTRIB_REF }}"
          TYPES="Compute/containerImages/containerImages.yaml Compute/containers/containers.yaml Compute/persistentVolumes/persistentVolumes.yaml Compute/routes/routes.yaml Data/postgreSqlDatabases/postgreSqlDatabases.yaml Data/mySqlDatabases/mySqlDatabases.yaml Security/secrets/secrets.yaml"
          for TYPE_YAML in $TYPES; do
            echo "Registering $TYPE_YAML..."
            curl -fsSL "$REPO_RAW/$TYPE_YAML" -o /tmp/type.yaml
            rad resource-type create -f /tmp/type.yaml || \
              (echo "Retrying after 5s..." && sleep 5 && rad resource-type create -f /tmp/type.yaml)
          done
          echo "✅ Resource types registered"

      - name: Register terraform recipes from resource-types-contrib
        run: |
          REPO="\${{ env.RESOURCE_TYPES_CONTRIB_REPO }}"
          REF="\${{ env.RESOURCE_TYPES_CONTRIB_REF }}"

          rad recipe register default \
            --environment "$ENVIRONMENT" \
            --resource-type Radius.Compute/containerImages \
            --template-kind terraform \
            --template-path "git::$REPO//Compute/containerImages/recipes/kubernetes/terraform?ref=$REF"

          rad recipe register default \
            --environment "$ENVIRONMENT" \
            --resource-type Radius.Compute/containers \
            --template-kind terraform \
            --template-path "git::$REPO//Compute/containers/recipes/kubernetes/terraform?ref=$REF"

          rad recipe register default \
            --environment "$ENVIRONMENT" \
            --resource-type Radius.Compute/persistentVolumes \
            --template-kind terraform \
            --template-path "git::$REPO//Compute/persistentVolumes/recipes/kubernetes/terraform?ref=$REF"

          rad recipe register default \
            --environment "$ENVIRONMENT" \
            --resource-type Radius.Compute/routes \
            --template-kind terraform \
            --template-path "git::$REPO//Compute/routes/recipes/kubernetes/terraform?ref=$REF"

          rad recipe register default \
            --environment "$ENVIRONMENT" \
            --resource-type Radius.Data/postgreSqlDatabases \
            --template-kind terraform \
            --template-path "git::$REPO//Data/postgreSqlDatabases/recipes/kubernetes/terraform?ref=$REF"

          rad recipe register default \
            --environment "$ENVIRONMENT" \
            --resource-type Radius.Data/mySqlDatabases \
            --template-kind terraform \
            --template-path "git::https://github.com/Reshrahim/terraform.git//aws/mysql" \
            --parameters vpcId="\${{ vars.RADIUS_VPC_ID }}" \
            --parameters 'subnetIds=\${{ vars.RADIUS_SUBNET_IDS }}'

          rad recipe register default \
            --environment "$ENVIRONMENT" \
            --resource-type Radius.Security/secrets \
            --template-kind terraform \
            --template-path "git::$REPO//Security/secrets/recipes/kubernetes/terraform?ref=$REF"

      - name: Verify app namespace on target cluster
        run: |
          TARGET_KUBECONFIG="$HOME/.kube/target-cluster"
          if [ -f "$TARGET_KUBECONFIG" ]; then
            BICEP_APP_NAME=$(grep -oP "name:\\s*'\\K[^']+" "$APP_FILE" | head -1 || echo "$APP_NAME")
            APP_NS="default-$BICEP_APP_NAME"
            echo "Ensuring namespace $APP_NS exists on target cluster..."
            kubectl --kubeconfig "$TARGET_KUBECONFIG" get namespace "$APP_NS" 2>/dev/null || \
              kubectl --kubeconfig "$TARGET_KUBECONFIG" create namespace "$APP_NS"
          fi

      - name: Deploy application
        run: |
          # Refresh the target kubeconfig secret with a fresh EKS token before deploying.
          if [ -n "\${{ vars.AWS_IAM_ROLE_ARN }}" ] && [ -n "\${{ vars.RADIUS_K8S_CLUSTER }}" ]; then
            echo "Refreshing EKS token for deployment..."
            CLUSTER="\${{ vars.RADIUS_K8S_CLUSTER }}"
            REGION="\${{ vars.AWS_REGION }}"
            ENDPOINT=$(aws eks describe-cluster --name "$CLUSTER" --region "$REGION" --query 'cluster.endpoint' --output text)
            CA_DATA=$(aws eks describe-cluster --name "$CLUSTER" --region "$REGION" --query 'cluster.certificateAuthority.data' --output text)
            TOKEN=$(aws eks get-token --cluster-name "$CLUSTER" --region "$REGION" --output json | jq -r '.status.token')
            TARGET_KUBECONFIG="/tmp/fresh-kubeconfig"
            printf 'apiVersion: v1\\nclusters:\\n- cluster:\\n    certificate-authority-data: %s\\n    server: %s\\n  name: eks\\ncontexts:\\n- context:\\n    cluster: eks\\n    user: eks-user\\n  name: eks\\ncurrent-context: eks\\nkind: Config\\nusers:\\n- name: eks-user\\n  user:\\n    token: %s\\n' "$CA_DATA" "$ENDPOINT" "$TOKEN" > "$TARGET_KUBECONFIG"
            kubectl delete secret target-kubeconfig -n radius-system --ignore-not-found
            kubectl create secret generic target-kubeconfig --namespace radius-system --from-file=config="$TARGET_KUBECONFIG"
            echo "Token refreshed, restarting applications-rp..."
            kubectl rollout restart deployment/applications-rp -n radius-system
            kubectl rollout restart deployment/dynamic-rp -n radius-system
            kubectl rollout status deployment/applications-rp -n radius-system --timeout=300s
            kubectl rollout status deployment/dynamic-rp -n radius-system --timeout=300s
          fi

          echo "Deploying $APP_FILE to environment $ENVIRONMENT..."
          DEPLOY_PARAMS=""
          if [ -n "$APP_IMAGE" ]; then
            DEPLOY_PARAMS="--parameters image=$APP_IMAGE"
          fi
          if [ -n "\${{ secrets.RADIUS_DB_PASSWORD }}" ]; then
            DEPLOY_PARAMS="$DEPLOY_PARAMS --parameters password=\${{ secrets.RADIUS_DB_PASSWORD }}"
          fi
          rad deploy "$APP_FILE" --environment "$ENVIRONMENT" $DEPLOY_PARAMS
          echo ""
          echo "✅ Deployment complete."

      - name: Show application status
        if: always()
        run: |
          rad app list || true

      - name: List Kubernetes secrets on target cluster
        if: always()
        run: |
          TARGET_KUBECONFIG="$HOME/.kube/target-cluster"
          BICEP_APP_NAME=$(grep -oP "name:\\s*'\\K[^']+" "$APP_FILE" | head -1 || echo "$APP_NAME")
          APP_NS="default-$BICEP_APP_NAME"
          if [ -f "$TARGET_KUBECONFIG" ]; then
            echo "=== Secrets in $APP_NS namespace ==="
            kubectl --kubeconfig "$TARGET_KUBECONFIG" get secrets -n "$APP_NS" -o wide 2>&1 || true
            echo ""
            echo "=== Secret details ==="
            for secret in $(kubectl --kubeconfig "$TARGET_KUBECONFIG" get secrets -n "$APP_NS" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null); do
              echo "--- $secret ---"
              kubectl --kubeconfig "$TARGET_KUBECONFIG" get secret "$secret" -n "$APP_NS" -o jsonpath='{.data}' 2>&1 | jq -r 'to_entries[] | "\\(.key): \\(.value | @base64d)"' 2>&1 || true
              echo ""
            done
          fi
          echo "=== Secrets on k3d cluster ==="
          kubectl get secrets -n "$APP_NS" -o wide 2>&1 || true

      - name: Debug resource locations
        if: always()
        run: |
          TARGET_KUBECONFIG="$HOME/.kube/target-cluster"
          BICEP_APP_NAME=$(grep -oP "name:\\s*'\\K[^']+" "$APP_FILE" | head -1 || echo "$APP_NAME")
          APP_NS="default-$BICEP_APP_NAME"
          echo "=== All resources on EKS ($APP_NS) ==="
          if [ -f "$TARGET_KUBECONFIG" ]; then
            kubectl --kubeconfig "$TARGET_KUBECONFIG" get all -n "$APP_NS" 2>&1 || true
            echo ""
            echo "=== All namespaces on EKS ==="
            kubectl --kubeconfig "$TARGET_KUBECONFIG" get namespaces 2>&1 || true
          fi
          echo ""
          echo "=== All resources on k3d ($APP_NS) ==="
          kubectl get all -n "$APP_NS" 2>&1 || true
          echo ""
          echo "=== dynamic-rp env vars ==="
          kubectl exec -n radius-system deploy/dynamic-rp -c dynamic-rp -- env 2>&1 | grep -E 'AWS_|KUBE_CONFIG|RADIUS_TARGET' || true
          echo ""
          echo "=== Terraform state secrets on k3d ==="
          kubectl get secrets -n radius-system -l "app.kubernetes.io/managed-by=terraform" -o wide 2>&1 || true

      - name: Collect Radius logs
        if: always()
        run: |
          mkdir -p /tmp/radius-logs
          for deploy in applications-rp dynamic-rp bicep-de controller ucpd; do
            echo "=== $deploy ===" >> /tmp/radius-logs/pods.txt
            kubectl logs -n radius-system -l app.kubernetes.io/name=$deploy --tail=200 >> /tmp/radius-logs/$deploy.log 2>&1 || true
            kubectl describe pods -n radius-system -l app.kubernetes.io/name=$deploy >> /tmp/radius-logs/$deploy-describe.txt 2>&1 || true
          done
          kubectl get pods -n radius-system -o wide >> /tmp/radius-logs/pods.txt 2>&1 || true
          kubectl get events -n radius-system --sort-by=.lastTimestamp >> /tmp/radius-logs/events.txt 2>&1 || true

      - name: Upload Radius logs
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: radius-logs
          path: /tmp/radius-logs/
          retention-days: 3

      - name: Cleanup control plane cluster
        if: always()
        run: k3d cluster delete radius-cp || true
`;
