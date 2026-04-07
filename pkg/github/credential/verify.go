/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package credential

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"text/template"
	"time"

	"github.com/radius-project/radius/pkg/github/environment"
)

const verifyWorkflowFilename = ".github/workflows/radius-verify-credentials.yml"
const deployWorkflowFilename = ".github/workflows/radius-deploy.yml"

// verifyBranchName is the dedicated branch for verification workflows.
// Using a separate branch keeps the main branch clean.
const verifyBranchName = "radius/verify-credentials"

// Verifier creates and triggers a GitHub Actions workflow that verifies cloud
// provider access using the credentials stored in the GitHub Environment.
type Verifier interface {
	// CommitVerificationWorkflow commits the credential verification workflow
	// file to the repository.
	CommitVerificationWorkflow(ctx context.Context, owner, repo, provider, envName string) error

	// TriggerVerification dispatches the verification workflow and returns
	// immediately. Use GetVerificationStatus to poll for completion.
	TriggerVerification(ctx context.Context, owner, repo, envName string) error

	// GetVerificationStatus returns the latest status of the verification
	// workflow run for the given environment.
	GetVerificationStatus(ctx context.Context, owner, repo, envName string) (*VerificationResult, error)

	// CommitDeployWorkflow commits a Radius deploy workflow to the repository's
	// default branch. This workflow runs `rad deploy` on push or manual trigger.
	CommitDeployWorkflow(ctx context.Context, owner, repo, provider, envName string) error

	// CommitAppCreateWorkflow commits the deploy workflow (reuses the deploy
	// workflow for application deployments).
	CommitAppCreateWorkflow(ctx context.Context, owner, repo, provider, envName string) error

	// TriggerAppDeploy dispatches the deploy workflow with the given app file.
	TriggerAppDeploy(ctx context.Context, owner, repo, envName, appFile string) error

	// GetAppDeployStatus returns the latest status of the deploy workflow run.
	GetAppDeployStatus(ctx context.Context, owner, repo, envName string) (*VerificationResult, error)

	// ListDeployments returns recent deploy workflow runs for the repository.
	ListDeployments(ctx context.Context, owner, repo string, limit int) ([]DeploymentSummary, error)

	// CreateAppFile creates a minimal Radius Bicep application file in the
	// repository if it does not already exist. Returns true if the file was created.
	CreateAppFile(ctx context.Context, owner, repo, filename string) (bool, error)

	// CheckAppFile checks if a Bicep application file exists in the repository.
	CheckAppFile(ctx context.Context, owner, repo, filename string) (bool, error)
}

type verifier struct {
	tokenSource environment.TokenSource
	httpClient  *http.Client
	baseURL     string
}

// NewVerifier creates a Verifier that uses GitHub API to manage verification workflows.
func NewVerifier(tokenSource environment.TokenSource) Verifier {
	return &verifier{
		tokenSource: tokenSource,
		httpClient:  http.DefaultClient,
		baseURL:     "https://api.github.com",
	}
}

// NewVerifierWithOptions creates a Verifier with custom HTTP client and base URL (for testing).
func NewVerifierWithOptions(tokenSource environment.TokenSource, httpClient *http.Client, baseURL string) Verifier {
	return &verifier{
		tokenSource: tokenSource,
		httpClient:  httpClient,
		baseURL:     baseURL,
	}
}

func (v *verifier) CommitVerificationWorkflow(ctx context.Context, owner, repo, provider, envName string) error {
	content, err := generateVerificationWorkflow(provider, envName)
	if err != nil {
		return fmt.Errorf("failed to generate verification workflow: %w", err)
	}

	// Ensure the dedicated orphan branch exists (used as the dispatch ref).
	if err := v.ensureBranch(ctx, owner, repo); err != nil {
		return fmt.Errorf("failed to create verification branch: %w", err)
	}

	// Commit to both branches:
	// - default branch: so GitHub indexes the workflow for dispatch
	// - orphan branch: so the dispatch ref has the workflow file
	defaultBranch, err := v.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get default branch: %w", err)
	}

	if err := v.commitFile(ctx, owner, repo, verifyWorkflowFilename,
		"radius setup", content, defaultBranch); err != nil {
		return fmt.Errorf("failed to commit to default branch: %w", err)
	}

	return v.commitFile(ctx, owner, repo, verifyWorkflowFilename,
		"radius setup", content, verifyBranchName)
}

func (v *verifier) TriggerVerification(ctx context.Context, owner, repo, envName string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/dispatches",
		v.baseURL, owner, repo, "radius-verify-credentials.yml")

	payload := map[string]interface{}{
		"ref": verifyBranchName,
		"inputs": map[string]string{
			"environment": envName,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal dispatch payload: %w", err)
	}

	// Retry — GitHub may need a moment to index the newly committed workflow.
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(3+attempt*2) * time.Second):
			}
		}
		_, lastErr = v.doRequest(ctx, http.MethodPost, url, body)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to trigger verification: %w", lastErr)
}

func (v *verifier) GetVerificationStatus(ctx context.Context, owner, repo, envName string) (*VerificationResult, error) {
	// List recent workflow runs for the verification workflow.
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?per_page=1",
		v.baseURL, owner, repo, "radius-verify-credentials.yml")

	respBody, err := v.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get verification runs: %w", err)
	}

	var result struct {
		WorkflowRuns []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
		} `json:"workflow_runs"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse workflow runs: %w", err)
	}

	if len(result.WorkflowRuns) == 0 {
		return &VerificationResult{
			Status:  "pending",
			Message: "No verification workflow runs found. Trigger a verification first.",
		}, nil
	}

	run := result.WorkflowRuns[0]
	vr := &VerificationResult{
		WorkflowRunURL: run.HTMLURL,
	}

	switch {
	case run.Status == "completed" && run.Conclusion == "success":
		vr.Status = "success"
		vr.Message = "Cloud credentials verified successfully."
	case run.Status == "completed" && run.Conclusion == "failure":
		vr.Status = "failure"
		vr.Message = "Credential verification failed. Check the workflow run for details."
	case run.Status == "completed":
		vr.Status = "failure"
		vr.Message = fmt.Sprintf("Verification completed with conclusion: %s", run.Conclusion)
	default:
		vr.Status = "in_progress"
		vr.Message = fmt.Sprintf("Verification is %s.", run.Status)
	}

	return vr, nil
}

// ensureBranch creates an orphan branch for verification workflows if it doesn't exist.
// An orphan branch has no parent commits and no shared history with the main branch.
func (v *verifier) ensureBranch(ctx context.Context, owner, repo string) error {
	// Check if the branch already exists.
	refURL := fmt.Sprintf("%s/repos/%s/%s/git/ref/heads/%s", v.baseURL, owner, repo, verifyBranchName)
	if _, err := v.doRequest(ctx, http.MethodGet, refURL, nil); err == nil {
		return nil // branch exists
	}

	// Create a blob with a placeholder README.
	blobURL := fmt.Sprintf("%s/repos/%s/%s/git/blobs", v.baseURL, owner, repo)
	blobPayload, _ := json.Marshal(map[string]string{
		"content":  "# Radius Verification\n\nThis branch is used by Radius to verify cloud credentials.\nDo not merge this branch.\n",
		"encoding": "utf-8",
	})
	blobResp, err := v.doRequest(ctx, http.MethodPost, blobURL, blobPayload)
	if err != nil {
		return fmt.Errorf("failed to create blob: %w", err)
	}
	var blob struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(blobResp, &blob); err != nil {
		return fmt.Errorf("failed to parse blob response: %w", err)
	}

	// Create a tree containing the README.
	treeURL := fmt.Sprintf("%s/repos/%s/%s/git/trees", v.baseURL, owner, repo)
	treePayload, _ := json.Marshal(map[string]interface{}{
		"tree": []map[string]string{
			{
				"path": "README.md",
				"mode": "100644",
				"type": "blob",
				"sha":  blob.SHA,
			},
		},
	})
	treeResp, err := v.doRequest(ctx, http.MethodPost, treeURL, treePayload)
	if err != nil {
		return fmt.Errorf("failed to create tree: %w", err)
	}
	var tree struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(treeResp, &tree); err != nil {
		return fmt.Errorf("failed to parse tree response: %w", err)
	}

	// Create an orphan commit (no parents).
	commitURL := fmt.Sprintf("%s/repos/%s/%s/git/commits", v.baseURL, owner, repo)
	commitPayload, _ := json.Marshal(map[string]interface{}{
		"message": "Initialize Radius verification branch",
		"tree":    tree.SHA,
		"parents": []string{},
	})
	commitResp, err := v.doRequest(ctx, http.MethodPost, commitURL, commitPayload)
	if err != nil {
		return fmt.Errorf("failed to create orphan commit: %w", err)
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal(commitResp, &commit); err != nil {
		return fmt.Errorf("failed to parse commit response: %w", err)
	}

	// Create the branch ref pointing to the orphan commit.
	createPayload, _ := json.Marshal(map[string]string{
		"ref": "refs/heads/" + verifyBranchName,
		"sha": commit.SHA,
	})
	createURL := fmt.Sprintf("%s/repos/%s/%s/git/refs", v.baseURL, owner, repo)
	_, err = v.doRequest(ctx, http.MethodPost, createURL, createPayload)
	return err
}

// commitFile creates or updates a file in the repository via the GitHub Contents API.
func (v *verifier) commitFile(ctx context.Context, owner, repo, path, message string, content []byte, branch string) error {
	// Check if file already exists to get its SHA (required for updates).
	getURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", v.baseURL, owner, repo, path, branch)
	var existingSHA string
	if respBody, err := v.doRequest(ctx, http.MethodGet, getURL, nil); err == nil {
		var existing struct {
			SHA string `json:"sha"`
		}
		if json.Unmarshal(respBody, &existing) == nil {
			existingSHA = existing.SHA
		}
	}

	putURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", v.baseURL, owner, repo, path)
	payload := map[string]interface{}{
		"message": message,
		"content": encodeBase64(content),
		"branch":  branch,
	}
	if existingSHA != "" {
		payload["sha"] = existingSHA
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal commit payload: %w", err)
	}

	_, err = v.doRequest(ctx, http.MethodPut, putURL, body)
	return err
}

func (v *verifier) doRequest(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	token, err := v.tokenSource.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub API error: %s %s returned %d: %s",
			method, url, resp.StatusCode, truncate(string(respBody), 200))
	}

	return respBody, nil
}

func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (v *verifier) CommitDeployWorkflow(ctx context.Context, owner, repo, provider, envName string) error {
	content, err := generateDeployWorkflow(provider, envName)
	if err != nil {
		return fmt.Errorf("failed to generate deploy workflow: %w", err)
	}

	defaultBranch, err := v.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get default branch: %w", err)
	}

	return v.commitFile(ctx, owner, repo, deployWorkflowFilename,
		"radius setup", content, defaultBranch)
}

func (v *verifier) CommitAppCreateWorkflow(ctx context.Context, owner, repo, provider, envName string) error {
	// Reuse the deploy workflow — application deployment is just `rad deploy` on an app.bicep.
	return v.CommitDeployWorkflow(ctx, owner, repo, provider, envName)
}

func (v *verifier) TriggerAppDeploy(ctx context.Context, owner, repo, envName, appFile string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/dispatches",
		v.baseURL, owner, repo, "radius-deploy.yml")

	defaultBranch, err := v.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get default branch: %w", err)
	}

	payload := map[string]interface{}{
		"ref": defaultBranch,
		"inputs": map[string]string{
			"environment": envName,
			"app_file":    appFile,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal dispatch payload: %w", err)
	}

	// Retry — GitHub may need a moment to index the newly committed workflow.
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(3+attempt*2) * time.Second):
			}
		}
		_, lastErr = v.doRequest(ctx, http.MethodPost, url, body)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to trigger app deployment: %w", lastErr)
}

func (v *verifier) GetAppDeployStatus(ctx context.Context, owner, repo, envName string) (*VerificationResult, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?per_page=1",
		v.baseURL, owner, repo, "radius-deploy.yml")

	respBody, err := v.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get deploy runs: %w", err)
	}

	var result struct {
		WorkflowRuns []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HTMLURL    string `json:"html_url"`
		} `json:"workflow_runs"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse workflow runs: %w", err)
	}

	if len(result.WorkflowRuns) == 0 {
		return &VerificationResult{
			Status:  "pending",
			Message: "No deploy workflow runs found.",
		}, nil
	}

	run := result.WorkflowRuns[0]
	vr := &VerificationResult{
		WorkflowRunURL: run.HTMLURL,
	}

	switch {
	case run.Status == "completed" && run.Conclusion == "success":
		vr.Status = "success"
		vr.Message = "Application deployed successfully."
	case run.Status == "completed" && run.Conclusion == "failure":
		vr.Status = "failure"
		vr.Message = "Deployment failed. Check the workflow run for details."
	case run.Status == "completed":
		vr.Status = "failure"
		vr.Message = fmt.Sprintf("Deployment completed with conclusion: %s", run.Conclusion)
	default:
		vr.Status = "in_progress"
		vr.Message = fmt.Sprintf("Deployment is %s.", run.Status)
	}

	return vr, nil
}

func (v *verifier) ListDeployments(ctx context.Context, owner, repo string, limit int) ([]DeploymentSummary, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	// Fetch more than needed since we filter to successful runs only.
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/runs?per_page=20&status=completed",
		v.baseURL, owner, repo, "radius-deploy.yml")

	respBody, err := v.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list deploy runs: %w", err)
	}

	var result struct {
		WorkflowRuns []struct {
			ID           int64  `json:"id"`
			Status       string `json:"status"`
			Conclusion   string `json:"conclusion"`
			HTMLURL      string `json:"html_url"`
			CreatedAt    string `json:"created_at"`
			HeadBranch   string `json:"head_branch"`
			DisplayTitle string `json:"display_title"`
		} `json:"workflow_runs"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse workflow runs: %w", err)
	}

	deployments := make([]DeploymentSummary, 0, limit)
	for _, run := range result.WorkflowRuns {
		if run.Conclusion != "success" {
			continue
		}
		deployments = append(deployments, DeploymentSummary{
			ID:         run.ID,
			Status:     run.Status,
			Conclusion: run.Conclusion,
			HTMLURL:    run.HTMLURL,
			CreatedAt:  run.CreatedAt,
			HeadBranch: run.HeadBranch,
			AppFile:    run.DisplayTitle,
		})
		if len(deployments) >= limit {
			break
		}
	}

	return deployments, nil
}

// defaultAppBicepTemplate is a minimal Radius application Bicep file.
const defaultAppBicepTemplate = `extension radius

@description('The Radius Application ID. Injected automatically by the rad CLI.')
param application string

resource demo 'Applications.Core/containers@2023-10-01-preview' = {
  name: '{{.AppName}}'
  properties: {
    application: application
    container: {
      image: 'ghcr.io/radius-project/samples/demo:latest'
      ports: {
        web: {
          containerPort: 3000
        }
      }
    }
  }
}
`

func (v *verifier) CreateAppFile(ctx context.Context, owner, repo, filename string) (bool, error) {
	if filename == "" {
		return false, fmt.Errorf("filename is required")
	}

	defaultBranch, err := v.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return false, fmt.Errorf("failed to get default branch: %w", err)
	}

	// Check if the file already exists.
	getURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", v.baseURL, owner, repo, filename, defaultBranch)
	if _, err := v.doRequest(ctx, http.MethodGet, getURL, nil); err == nil {
		// File already exists — nothing to do.
		return false, nil
	}

	// Derive app name from filename (strip .bicep extension and path).
	appName := filename
	if idx := len(appName) - len(".bicep"); idx > 0 && appName[idx:] == ".bicep" {
		appName = appName[:idx]
	}
	// Strip any directory prefix.
	for i := len(appName) - 1; i >= 0; i-- {
		if appName[i] == '/' {
			appName = appName[i+1:]
			break
		}
	}
	if appName == "" {
		appName = "app"
	}

	tmpl, err := template.New("app").Parse(defaultAppBicepTemplate)
	if err != nil {
		return false, fmt.Errorf("failed to parse app template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{"AppName": appName}); err != nil {
		return false, fmt.Errorf("failed to render app template: %w", err)
	}

	if err := v.commitFile(ctx, owner, repo, filename,
		fmt.Sprintf("Add Radius application: %s", filename),
		buf.Bytes(), defaultBranch); err != nil {
		return false, fmt.Errorf("failed to commit app file: %w", err)
	}

	return true, nil
}

func (v *verifier) CheckAppFile(ctx context.Context, owner, repo, filename string) (bool, error) {
	if filename == "" {
		filename = "app.bicep"
	}

	defaultBranch, err := v.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return false, fmt.Errorf("failed to get default branch: %w", err)
	}

	getURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", v.baseURL, owner, repo, filename, defaultBranch)
	if _, err := v.doRequest(ctx, http.MethodGet, getURL, nil); err == nil {
		return true, nil
	}

	return false, nil
}

func (v *verifier) getDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", v.baseURL, owner, repo)
	respBody, err := v.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get repo info: %w", err)
	}
	var repoInfo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(respBody, &repoInfo); err != nil {
		return "", fmt.Errorf("failed to parse repo info: %w", err)
	}
	if repoInfo.DefaultBranch == "" {
		return "main", nil
	}
	return repoInfo.DefaultBranch, nil
}

// generateVerificationWorkflow creates a GitHub Actions workflow YAML that
// tests whether the credentials in the GitHub Environment can access the
// target cloud provider.
func generateVerificationWorkflow(provider, envName string) ([]byte, error) {
	tmpl, err := template.New("verify").Parse(verificationWorkflowTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workflow template: %w", err)
	}

	data := struct {
		Provider    string
		Environment string
	}{
		Provider:    provider,
		Environment: envName,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute workflow template: %w", err)
	}
	return buf.Bytes(), nil
}

// defaultRunnerImage is the default container image used by Radius workflows.
// Users can override this by building and pushing their own image from
// deploy/images/radius-runner/Dockerfile.
const defaultRunnerImage = "ghcr.io/sk593/radius-runner:latest"

const verificationWorkflowTemplate = `# This workflow is auto-generated by Radius to verify cloud credentials.
# It uses the credentials stored in the GitHub Environment to authenticate
# with the cloud provider and run a simple verification command.
name: Radius - Verify Cloud Credentials

on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'GitHub Environment name'
        required: true
        default: '{{ .Environment }}'

permissions:
  id-token: write
  contents: read

jobs:
  verify:
    name: Verify {{ .Provider }} credentials
    runs-on: ubuntu-latest
    environment: ${{"{{"}} inputs.environment {{"}}"}}
    steps:
      - name: Checkout
        uses: actions/checkout@v4
{{ if eq .Provider "azure" }}
      - name: Azure Login (OIDC)
        uses: azure/login@v2
        with:
          client-id: ${{"{{"}} vars.AZURE_CLIENT_ID {{"}}"}}
          tenant-id: ${{"{{"}} vars.AZURE_TENANT_ID {{"}}"}}
          subscription-id: ${{"{{"}} vars.AZURE_SUBSCRIPTION_ID {{"}}"}}

      - name: Verify Azure access
        run: |
          set -euo pipefail
          echo "Verifying Azure access..."
          az account show --query '{name:name, state:state}' --output table
          echo ""
          echo "✅ Azure credentials are working correctly."
{{ else if eq .Provider "aws" }}
      - name: Configure AWS Credentials (OIDC)
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: ${{"{{"}} vars.AWS_IAM_ROLE_ARN {{"}}"}}
          aws-region: ${{"{{"}} vars.AWS_REGION {{"}}"}}

      - name: Verify AWS access
        run: |
          set -euo pipefail
          echo "Verifying AWS access..."
          CALLER_ARN=$(aws sts get-caller-identity --query 'Arn' --output text)
          echo "${CALLER_ARN}" | sed 's/[0-9]\{12\}/****/g'
          echo ""
          echo "✅ AWS credentials are working correctly."
{{ end }}
`

func generateDeployWorkflow(provider, envName string) ([]byte, error) {
	tmpl, err := template.New("deploy").Parse(deployWorkflowTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse deploy workflow template: %w", err)
	}

	data := struct {
		Provider    string
		Environment string
	}{
		Provider:    provider,
		Environment: envName,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("failed to execute deploy workflow template: %w", err)
	}
	return buf.Bytes(), nil
}

const deployWorkflowTemplate = `# This workflow is auto-generated by Radius to deploy applications.
# It spins up an ephemeral k3d cluster, installs Radius, and runs rad deploy.
# No pre-existing Radius installation is required on your cluster.
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
        default: '{{ .Environment }}'
      app_file:
        description: 'Bicep application file to deploy'
        required: true
        default: 'app.bicep'

permissions:
  id-token: write
  contents: read

env:
  ENVIRONMENT: ${{"{{"}} inputs.environment || '{{ .Environment }}' {{"}}"}}
  APP_FILE: ${{"{{"}} inputs.app_file || 'app.bicep' {{"}}"}}

jobs:
  deploy:
    name: Deploy with Radius
    runs-on: ubuntu-latest
    environment: ${{"{{"}} inputs.environment || '{{ .Environment }}' {{"}}"}}
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Install k3d
        run: curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash

      - name: Create ephemeral cluster
        run: |
          k3d cluster create radius-ephemeral --wait
          kubectl wait --for=condition=Ready node --all --timeout=120s

      - name: Install Radius CLI
        run: |
          wget -q "https://raw.githubusercontent.com/radius-project/radius/main/deploy/install.sh" -O - | /bin/bash
          rad version

      - name: Install Radius on cluster
        run: |
          rad install kubernetes --set rp.publicEndpointOverride=localhost
          kubectl wait --for=condition=Available deployment --all -n radius-system --timeout=300s

      - name: Configure Radius environment
        run: |
          rad workspace create kubernetes default
          rad group create default
          rad group switch default
          rad env create "$ENVIRONMENT" --namespace default
          rad env switch "$ENVIRONMENT"
{{ if eq .Provider "azure" }}
      - name: Azure Login (OIDC)
        uses: azure/login@v2
        with:
          client-id: ${{"{{"}} vars.AZURE_CLIENT_ID {{"}}"}}
          tenant-id: ${{"{{"}} vars.AZURE_TENANT_ID {{"}}"}}
          subscription-id: ${{"{{"}} vars.AZURE_SUBSCRIPTION_ID {{"}}"}}

      - name: Register Azure credentials with Radius
        run: |
          rad credential register azure wi \
            --client-id "${{"{{"}} vars.AZURE_CLIENT_ID {{"}}"}}" \
            --tenant-id "${{"{{"}} vars.AZURE_TENANT_ID {{"}}"}}"
          rad env update "$ENVIRONMENT" \
            --azure-subscription-id "${{"{{"}} vars.AZURE_SUBSCRIPTION_ID {{"}}"}}" \
            --azure-resource-group "${{"{{"}} vars.AZURE_RESOURCE_GROUP {{"}}"}}"
{{ else if eq .Provider "aws" }}
      - name: Configure AWS Credentials (OIDC)
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: ${{"{{"}} vars.AWS_IAM_ROLE_ARN {{"}}"}}
          aws-region: ${{"{{"}} vars.AWS_REGION {{"}}"}}

      - name: Register AWS credentials with Radius
        run: |
          rad credential register aws irsa \
            --iam-role "${{"{{"}} vars.AWS_IAM_ROLE_ARN {{"}}"}}"
          rad env update "$ENVIRONMENT" \
            --aws-region "${{"{{"}} vars.AWS_REGION {{"}}"}}" \
            --aws-account-id "${{"{{"}} vars.AWS_ACCOUNT_ID {{"}}"}}"
{{ end }}
      - name: Deploy application
        run: |
          echo "Deploying $APP_FILE to environment $ENVIRONMENT..."
          rad deploy "$APP_FILE" --environment "$ENVIRONMENT"
          echo ""
          echo "✅ Deployment complete."

      - name: Show application status
        if: always()
        run: |
          rad app list || true
          rad resource list --environment "$ENVIRONMENT" || true

      - name: Cleanup ephemeral cluster
        if: always()
        run: k3d cluster delete radius-ephemeral || true
`
