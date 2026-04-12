// Radius control plane API client for fetching live deployment state.
// Separate from github-api.ts to maintain separation of concerns.
// Used by the deployed application graph page (US7, P3).

import type { ApplicationGraphResponse, DeploymentStatus } from './graph-types.js';

export interface DeployedResourceStatus {
  resourceId: string;
  status: DeploymentStatus;
  portalUrl?: string;
  error?: string;
}

export interface DeployedGraphData {
  application: ApplicationGraphResponse;
  deploymentStatuses: DeployedResourceStatus[];
}

export class RadiusAPI {
  constructor(
    private baseUrl: string,
    private token?: string,
  ) {}

  /**
   * Fetch the application graph from a running Radius control plane.
   */
  async getApplicationGraph(appId: string): Promise<ApplicationGraphResponse | null> {
    const url = `${this.baseUrl}${appId}/getGraph`;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    try {
      const resp = await fetch(url, {
        method: 'POST',
        headers,
        body: JSON.stringify({}),
      });

      if (!resp.ok) return null;
      return (await resp.json()) as ApplicationGraphResponse;
    } catch {
      return null;
    }
  }

  /**
   * Fetch deployment status for all resources in an application.
   */
  async getDeploymentStatuses(appId: string): Promise<DeployedResourceStatus[]> {
    const graph = await this.getApplicationGraph(appId);
    if (!graph) return [];

    return graph.resources.map((r) => ({
      resourceId: r.id,
      status: mapProvisioningState(r.provisioningState),
      portalUrl: undefined, // Cloud portal URL would be derived from output resources.
      error: r.provisioningState === 'Failed' ? 'Resource deployment failed' : undefined,
    }));
  }
}

function mapProvisioningState(state: string): DeploymentStatus {
  switch (state) {
    case 'Succeeded':
      return 'success';
    case 'Failed':
      return 'failed';
    case 'Creating':
    case 'Updating':
    case 'Deleting':
    case 'Provisioning':
      return 'in-progress';
    case 'Accepted':
      return 'queued';
    default:
      return 'queued';
  }
}
