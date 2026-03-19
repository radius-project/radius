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

	"github.com/radius-project/radius/pkg/github/environment"
)

const verifyWorkflowFilename = ".github/workflows/radius-verify-credentials.yml"

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

	return v.commitFile(ctx, owner, repo, verifyWorkflowFilename,
		"Add Radius credential verification workflow", content)
}

func (v *verifier) TriggerVerification(ctx context.Context, owner, repo, envName string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/workflows/%s/dispatches",
		v.baseURL, owner, repo, "radius-verify-credentials.yml")

	payload := map[string]interface{}{
		"ref": "main",
		"inputs": map[string]string{
			"environment": envName,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal dispatch payload: %w", err)
	}

	_, err = v.doRequest(ctx, http.MethodPost, url, body)
	return err
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

// commitFile creates or updates a file in the repository via the GitHub Contents API.
func (v *verifier) commitFile(ctx context.Context, owner, repo, path, message string, content []byte) error {
	// Check if file already exists to get its SHA (required for updates).
	getURL := fmt.Sprintf("%s/repos/%s/%s/contents/%s", v.baseURL, owner, repo, path)
	var existingSHA string
	if respBody, err := v.doRequest(ctx, http.MethodGet, getURL, nil); err == nil {
		var existing struct {
			SHA string `json:"sha"`
		}
		if json.Unmarshal(respBody, &existing) == nil {
			existingSHA = existing.SHA
		}
	}

	payload := map[string]interface{}{
		"message": message,
		"content": encodeBase64(content),
	}
	if existingSHA != "" {
		payload["sha"] = existingSHA
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal commit payload: %w", err)
	}

	_, err = v.doRequest(ctx, http.MethodPut, getURL, body)
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
          echo "Verifying Azure access..."
          az account show --output table
          echo ""
          echo "Listing resource groups..."
          az group list --output table --query "[].{Name:name, Location:location}" || true
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
          echo "Verifying AWS access..."
          aws sts get-caller-identity
          echo ""
          echo "Listing S3 buckets..."
          aws s3 ls || true
          echo ""
          echo "✅ AWS credentials are working correctly."
{{ end }}
`
