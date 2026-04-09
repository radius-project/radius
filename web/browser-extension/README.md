# Setup with Radius — Browser Extension

A Chrome/Edge browser extension that adds a "Deploy" button to GitHub repository pages and provides a wizard for configuring OIDC credentials (AWS and Azure) for Radius deployments. The extension calls the GitHub REST API directly — no backend server is needed.

## Features

- **Deploy button** injected next to the Code button on GitHub repo pages
- **Application setup** — creates a Radius Bicep template in the repo
- **Environment creation** — configures AWS (IRSA) or Azure (Workload Identity) OIDC credentials as GitHub Environment variables
- **Credential verification** — triggers a GitHub Actions workflow to test cloud access
- **Environment dependencies** — configures Kubernetes cluster, namespace, OCI registry, VPC, and subnets
- **Deploy trigger** — dispatches a deploy workflow from the extension
- **GitHub App Device Flow** — OAuth authentication without a backend server

## Prerequisites

- A GitHub App with Device Flow enabled (see [Setup](#github-app-setup))
- Node.js 18+ and npm
- Chrome or Edge browser

## Build

```bash
cd web/browser-extension
npm install
npm run build
```

The built extension is in `dist/`.

## Load in Browser

### Chrome
1. Go to `chrome://extensions/`
2. Enable "Developer mode"
3. Click "Load unpacked"
4. Select the `dist/` folder

### Edge
1. Go to `edge://extensions/`
2. Enable "Developer mode"
3. Click "Load unpacked"
4. Select the `dist/` folder

## GitHub App Setup

1. Go to https://github.com/settings/apps/new
2. Fill in:
   - **Name**: e.g., `Radius Deploy`
   - **Homepage URL**: `https://github.com/radius-project/radius`
   - **Webhook**: Uncheck "Active" (not needed)
   - **Enable Device Flow**: Check this
3. **Repository permissions**:
   - Administration: Read & Write (required for creating environments)
   - Actions: Read & Write (trigger/poll workflows)
   - Contents: Read & Write (commit workflow files)
   - Environments: Read & Write (set environment variables)
   - Secrets: Read & Write (store encrypted secrets for SP auth)
   - Variables: Read & Write (set environment variables)
4. Click "Create GitHub App"
5. Copy the **Client ID** (starts with `Iv23li...`)
6. **Install the app** on your account/repos: go to `https://github.com/apps/YOUR-APP-SLUG/installations/new`

## Testing the Extension

### First-time setup

1. Load the extension in your browser (see [Load in Browser](#load-in-browser))
2. Navigate to a GitHub repository
3. Click the **Deploy** button (next to the Code button)
4. Enter the **App slug** and **Client ID** from your GitHub App, click **Continue**
5. Click **Install GitHub App** to install on your account
6. Click **Sign in with GitHub** — enter the device code at `github.com/login/device`

### Test: Define an Application

1. On the extension page, enter a Bicep filename (default: `app.bicep`)
2. Click **Define Application**
3. Verify: the file is committed to the repo

### Test: Create an AWS Environment

1. After defining an app, click **Create Environment**
2. Select the **AWS** tab
3. Enter:
   - Environment name: `dev`
   - IAM Role ARN: `arn:aws:iam::ACCOUNT:role/radius-deploy`
   - Region: select from dropdown
4. Click **Confirm authentication**
5. The extension creates the GitHub Environment, sets `AWS_IAM_ROLE_ARN` and `AWS_REGION` variables, commits the verification workflow, and triggers it
6. Verify: the verification workflow passes in the repo's Actions tab

### Test: Create an Azure Environment

1. After defining an app, click **Create Environment**
2. Select the **Azure** tab
3. Enter tenant ID, client ID, subscription ID
4. Choose **Workload Identity** (recommended)
5. Click **Confirm authentication**
6. Verify: GitHub Environment created with `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_SUBSCRIPTION_ID` variables

### Test: Configure Dependencies

1. After credential verification succeeds, the dependencies form appears
2. Enter:
   - Kubernetes cluster name (EKS or AKS cluster)
   - Kubernetes namespace (default: `default`)
   - OCI registry, VPC, subnets (optional)
3. Click **Save dependencies**
4. Verify: `RADIUS_K8S_CLUSTER`, `RADIUS_K8S_NAMESPACE`, etc. are set as GitHub Environment variables

### Test: Deploy Application

1. After dependencies, the deploy form appears
2. Verify the app file, app name, and environment name are pre-populated
3. Click **Deploy Application**
4. The extension commits/updates the deploy workflow and triggers it
5. Verify: the deploy workflow runs in the repo's Actions tab

### Test: Token Expiry

1. Wait 8+ hours (or manually clear `chrome.storage.local`)
2. Try any action — should show "GitHub token expired. Please sign in again."
3. Reload the extension — sign-in flow should appear

### Test: Fork Limitations

Note: GitHub App user tokens have **read-only Contents access on forked repos**. This means:
- Environment creation ✅ (works)
- Variable/secret setting ✅ (works)
- Workflow commits ❌ (fails with 403)
- Workflow dispatch ✅ (works if workflow already exists)

Test on a **non-fork repo** for the full flow.

## File Structure

```
web/browser-extension/
├── manifest.json                    # Manifest V3
├── package.json                     # Dependencies (tweetnacl)
├── tsconfig.json                    # TypeScript config
├── src/
│   ├── content/
│   │   ├── inject.ts                # Injects Deploy button on GitHub pages
│   │   └── styles.css               # Button + dropdown styling
│   ├── popup/
│   │   ├── popup.html               # Setup/deploy wizard
│   │   ├── popup.ts                 # Wizard logic, form handlers
│   │   └── popup.css                # Popup styling
│   ├── app-create/
│   │   ├── app-create.html          # Standalone deploy page
│   │   └── app-create.ts            # Deploy page logic
│   ├── background/
│   │   └── service-worker.ts        # Message passing, env status caching
│   └── shared/
│       ├── github-client.ts         # GitHub REST API client (environments, variables, secrets, workflows)
│       ├── api.ts                   # Token storage, client factory
│       ├── device-flow.ts           # GitHub App Device Flow OAuth
│       └── types.ts                 # TypeScript types
└── icons/
    ├── icon-16.png
    ├── icon-48.png
    └── icon-128.png
```

## Troubleshooting

| Issue | Solution |
|---|---|
| Deploy button doesn't appear | Make sure you're on the repo's main code page (not issues, PRs, etc.). The button only shows when the green "Code" button is visible. |
| "GitHub token expired" | Sign in again via the Device Flow. Tokens expire after 8 hours. |
| "Resource not accessible by integration" (403) | Check GitHub App permissions (Administration, Contents, Actions must be Read & Write). Accept updated permissions at `github.com/settings/installations`. |
| Workflow commits fail on forks | Use a non-fork repo. GitHub limits write access to fork contents via integrations. |
| "Bad credentials" (401) | Token expired or revoked. The extension auto-clears it — reload and sign in again. |
| Environment creation fails | Ensure the GitHub App has Administration: Read & Write permission. |
