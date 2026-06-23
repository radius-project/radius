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

// Package tfstate backs up and restores Terraform recipe state across ephemeral Radius
// control planes.
//
// Terraform recipes store their state in Kubernetes Secrets (the Terraform "kubernetes"
// backend), not in the Radius PostgreSQL databases. Those Secrets live in the radius-system
// namespace and are labelled "tfstate=true" by the backend. When the Radius control plane runs
// on an ephemeral cluster (for example a k3d cluster inside a CI runner), those Secrets are
// destroyed on teardown. Without backing them up, a second deploy of the same Terraform-backed
// resource in a later run plans from an empty backend and either fails or orphans cloud
// resources.
//
// This package exports those Secrets to a state directory (the same directory used for the
// PostgreSQL dumps) and restores them into a fresh cluster before any deploy runs.
package tfstate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
)

const (
	// DefaultNamespace is the Kubernetes namespace where the Radius control plane and its
	// Terraform state Secrets are installed.
	DefaultNamespace = "radius-system"

	// LabelSelector matches the Secrets that the Terraform Kubernetes backend creates for recipe
	// state. The backend labels every state Secret with "tfstate=true".
	// https://developer.hashicorp.com/terraform/language/settings/backends/kubernetes
	LabelSelector = "tfstate=true"

	// SubDir is the directory, relative to the state directory, where Terraform state Secrets are
	// written. It keeps the Terraform backups separate from the PostgreSQL dumps in the same tree.
	SubDir = "tfstate"
)

// Client backs up and restores Terraform recipe state stored as Kubernetes Secrets.
type Client struct {
	clientset k8s.Interface
	namespace string
}

// NewClient creates a Client backed by the supplied Kubernetes clientset and namespace.
func NewClient(clientset k8s.Interface, namespace string) *Client {
	return &Client{clientset: clientset, namespace: namespace}
}

// NewClientForContext builds a Client from a kubeconfig context name, targeting the given
// namespace.
func NewClientForContext(kubeContext, namespace string) (*Client, error) {
	clientset, _, err := kubernetes.NewClientset(kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return NewClient(clientset, namespace), nil
}

// Backup writes every Terraform state Secret in the namespace to <stateDir>/tfstate/<name>.json.
// Existing files in the target directory are removed first so that a Secret deleted since the
// previous backup does not linger and get restored.
func (c *Client) Backup(ctx context.Context, stateDir string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	secrets, err := c.clientset.CoreV1().Secrets(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: LabelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list Terraform state secrets: %w", err)
	}

	outDir := filepath.Join(stateDir, SubDir)
	// Start from a clean directory so removed secrets are not resurrected on restore.
	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("failed to clear Terraform state directory %q: %w", outDir, err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("failed to create Terraform state directory %q: %w", outDir, err)
	}

	for i := range secrets.Items {
		secret := sanitize(&secrets.Items[i])

		data, err := json.MarshalIndent(secret, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to serialize Terraform state secret %q: %w", secret.Name, err)
		}

		outPath := filepath.Join(outDir, secret.Name+".json")
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return fmt.Errorf("failed to write Terraform state file %q: %w", outPath, err)
		}

		logger.Info("Backed up Terraform state secret", "secret", secret.Name, "file", outPath)
	}

	logger.Info("Terraform state backup complete", "count", len(secrets.Items), "stateDir", outDir)
	return nil
}

// HasBackup reports whether any Terraform state backup files exist in the state directory.
func HasBackup(stateDir string) bool {
	entries, err := os.ReadDir(filepath.Join(stateDir, SubDir))
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			return true
		}
	}

	return false
}

// Restore re-creates the Terraform state Secrets from <stateDir>/tfstate into the namespace.
// It is idempotent: a Secret that already exists is updated in place. Restore must run before
// any deploy so that Terraform recipes plan against the restored backend.
func (c *Client) Restore(ctx context.Context, stateDir string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	inDir := filepath.Join(stateDir, SubDir)
	entries, err := os.ReadDir(inDir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("No Terraform state backup found, skipping restore", "stateDir", inDir)
			return nil
		}
		return fmt.Errorf("failed to read Terraform state directory %q: %w", inDir, err)
	}

	restored := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(inDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read Terraform state file %q: %w", path, err)
		}

		var secret corev1.Secret
		if err := json.Unmarshal(data, &secret); err != nil {
			return fmt.Errorf("failed to parse Terraform state file %q: %w", path, err)
		}

		// Force the namespace to the target in case the backup came from a differently-named one.
		secret.Namespace = c.namespace

		if err := c.applySecret(ctx, &secret); err != nil {
			return err
		}

		logger.Info("Restored Terraform state secret", "secret", secret.Name)
		restored++
	}

	logger.Info("Terraform state restore complete", "count", restored)
	return nil
}

// applySecret creates the Secret, or updates it in place if it already exists.
func (c *Client) applySecret(ctx context.Context, secret *corev1.Secret) error {
	secrets := c.clientset.CoreV1().Secrets(c.namespace)

	_, err := secrets.Create(ctx, secret, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create Terraform state secret %q: %w", secret.Name, err)
	}

	// The Secret already exists; update it in place. Carry over the current resourceVersion,
	// which the API server requires for updates.
	existing, getErr := secrets.Get(ctx, secret.Name, metav1.GetOptions{})
	if getErr != nil {
		return fmt.Errorf("failed to read existing Terraform state secret %q: %w", secret.Name, getErr)
	}

	secret.ResourceVersion = existing.ResourceVersion
	if _, err := secrets.Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("failed to update Terraform state secret %q: %w", secret.Name, err)
	}

	return nil
}

// sanitize returns a copy of the Secret with server-managed fields cleared so that it can be
// re-created cleanly in a different cluster.
func sanitize(secret *corev1.Secret) *corev1.Secret {
	cleaned := secret.DeepCopy()
	cleaned.ResourceVersion = ""
	cleaned.UID = ""
	cleaned.Generation = 0
	cleaned.CreationTimestamp = metav1.Time{}
	cleaned.DeletionTimestamp = nil
	cleaned.ManagedFields = nil
	cleaned.OwnerReferences = nil
	cleaned.SelfLink = ""
	return cleaned
}
