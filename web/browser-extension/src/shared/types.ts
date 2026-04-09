// Shared types matching the Go backend API (pkg/github/api/types.go).

export interface CreateAWSEnvironmentRequest {
  name: string;
  roleARN: string;
  region: string;
  accountID: string;
}

export interface CreateAzureEnvironmentRequest {
  name: string;
  tenantID: string;
  clientID: string;
  subscriptionID: string;
  resourceGroup?: string;
  authType: 'WorkloadIdentity' | 'ServicePrincipal';
  clientSecret?: string;
  azureAccessToken?: string;
}

export interface EnvironmentResponse {
  name: string;
  provider: string;
  githubEnvironmentCreated: boolean;
  variablesSet: string[];
  credentialsVerified: boolean;
  federatedCredentialCreated?: boolean;
}

export interface VerificationResponse {
  provider: string;
  status: 'pending' | 'in_progress' | 'success' | 'failure';
  message: string;
  workflowRunURL?: string;
}

export interface ErrorResponse {
  error: string;
}

export interface SaveDependenciesRequest {
  kubernetesCluster?: string;
  kubernetesNamespace?: string;
  ociRegistry?: string;
  vpc?: string;
  subnets?: string;
  resourceGroup?: string;
}

export interface DependenciesResponse {
  variablesSet: string[];
}

export interface DeployAppRequest {
  appFile: string;
}

export interface CreateAppFileRequest {
  filename: string;
}

export interface CreateAppFileResponse {
  filename: string;
  created: boolean;
}

export interface DeploymentSummary {
  id: number;
  status: string;
  conclusion: string;
  appFile?: string;
  environment?: string;
  htmlURL: string;
  createdAt: string;
  headBranch?: string;
}

export type CloudProvider = 'aws' | 'azure';

// AWS regions for the dropdown.
export const AWS_REGIONS = [
  'us-east-1',
  'us-east-2',
  'us-west-1',
  'us-west-2',
  'af-south-1',
  'ap-east-1',
  'ap-south-1',
  'ap-south-2',
  'ap-southeast-1',
  'ap-southeast-2',
  'ap-southeast-3',
  'ap-northeast-1',
  'ap-northeast-2',
  'ap-northeast-3',
  'ca-central-1',
  'eu-central-1',
  'eu-central-2',
  'eu-west-1',
  'eu-west-2',
  'eu-west-3',
  'eu-north-1',
  'eu-south-1',
  'eu-south-2',
  'me-south-1',
  'me-central-1',
  'sa-east-1',
] as const;
