// GitHub App Device Flow for browser extensions.
// Uses the Device Authorization Grant (RFC 8628) which doesn't require a client secret.
// https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/generating-a-user-access-token-for-a-github-app#using-the-device-flow-to-generate-a-user-access-token

const STORAGE_KEY_CLIENT_ID = 'radius_github_app_client_id';
const STORAGE_KEY_APP_SLUG = 'radius_github_app_slug';

const DEVICE_CODE_URL = 'https://github.com/login/device/code';
const ACCESS_TOKEN_URL = 'https://github.com/login/oauth/access_token';

export interface DeviceCodeResponse {
  device_code: string;
  user_code: string;
  verification_uri: string;
  expires_in: number;
  interval: number;
}

export interface DeviceFlowCallbacks {
  onUserCode: (userCode: string, verificationUri: string) => void;
  onPolling: () => void;
  onSuccess: (token: string) => void;
  onError: (error: string) => void;
}

// startDeviceFlow initiates the GitHub Device Flow and polls for completion.
// The caller provides callbacks for each stage of the flow.
export async function startDeviceFlow(callbacks: DeviceFlowCallbacks): Promise<void> {
  const clientId = await getClientID();
  if (!clientId) {
    callbacks.onError('GitHub App Client ID not configured. Enter it in Settings.');
    return;
  }

  // Step 1: Request a device code.
  const codeResp = await fetch(DEVICE_CODE_URL, {
    method: 'POST',
    headers: {
      'Accept': 'application/json',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      client_id: clientId,
    }),
  });

  if (!codeResp.ok) {
    callbacks.onError(`Failed to start device flow: HTTP ${codeResp.status}`);
    return;
  }

  const codeData: DeviceCodeResponse = await codeResp.json();

  // Step 2: Show the user code and open the verification URL.
  callbacks.onUserCode(codeData.user_code, codeData.verification_uri);

  // Step 3: Poll for the access token.
  const intervalMs = (codeData.interval || 5) * 1000;
  const expiresAt = Date.now() + codeData.expires_in * 1000;

  const poll = async (): Promise<void> => {
    if (Date.now() > expiresAt) {
      callbacks.onError('Device flow expired. Please try again.');
      return;
    }

    callbacks.onPolling();

    const tokenResp = await fetch(ACCESS_TOKEN_URL, {
      method: 'POST',
      headers: {
        'Accept': 'application/json',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        client_id: clientId,
        device_code: codeData.device_code,
        grant_type: 'urn:ietf:params:oauth:grant-type:device_code',
      }),
    });

    if (!tokenResp.ok) {
      callbacks.onError(`Token request failed: HTTP ${tokenResp.status}`);
      return;
    }

    const tokenData = await tokenResp.json();

    if (tokenData.error === 'authorization_pending') {
      // User hasn't authorized yet — keep polling.
      setTimeout(poll, intervalMs);
      return;
    }

    if (tokenData.error === 'slow_down') {
      // GitHub wants us to slow down — add 5 seconds.
      setTimeout(poll, intervalMs + 5000);
      return;
    }

    if (tokenData.error) {
      callbacks.onError(`Authorization failed: ${tokenData.error_description || tokenData.error}`);
      return;
    }

    if (tokenData.access_token) {
      callbacks.onSuccess(tokenData.access_token);
      return;
    }

    callbacks.onError('Unexpected response from GitHub');
  };

  setTimeout(poll, intervalMs);
}

// --- Storage helpers ---

export async function getClientID(): Promise<string> {
  const result = await chrome.storage.sync.get(STORAGE_KEY_CLIENT_ID);
  return (result[STORAGE_KEY_CLIENT_ID] as string) || '';
}

export async function setClientID(clientId: string): Promise<void> {
  await chrome.storage.sync.set({ [STORAGE_KEY_CLIENT_ID]: clientId });
}

export async function getAppSlug(): Promise<string> {
  const result = await chrome.storage.sync.get(STORAGE_KEY_APP_SLUG);
  return (result[STORAGE_KEY_APP_SLUG] as string) || '';
}

export async function setAppSlug(slug: string): Promise<void> {
  await chrome.storage.sync.set({ [STORAGE_KEY_APP_SLUG]: slug });
}
