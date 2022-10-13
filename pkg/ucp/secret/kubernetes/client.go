// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"

	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/secret"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreV1Types "k8s.io/client-go/kubernetes/typed/core/v1"
)

var _ secret.Client = (*Client)(nil)

type Client struct {
	KubernetesSecretClient coreV1Types.SecretInterface
}

func (c *Client) CreateOrUpdate(ctx context.Context, name string, secrets interface{}) error {
	objMetadata := metav1.ObjectMeta{Name: name}
	var secretObject v1.Secret
	switch v := secrets.(type) {
	case ucp.AzureServicePrincipalProperties:
		secretsData := map[string]string{
			"ClientId": *v.ClientID,
			"Secret":   *v.Secret,
			"TenantId": *v.TenantID,
		}
		secretObject = v1.Secret{
			ObjectMeta: objMetadata,
			Type:       "Opaque",
			StringData: secretsData,
		}
	}
	// ignore the response value as we don't want to handle secrets
	_, err := c.KubernetesSecretClient.Create(ctx, &secretObject, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) Delete(ctx context.Context, id string) error {
	return c.KubernetesSecretClient.Delete(ctx, id, metav1.DeleteOptions{})
}

func (c *Client) Get(ctx context.Context, id string) (string, error) {
	_, err := c.KubernetesSecretClient.Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return id, nil
}

func (c *Client) List(ctx context.Context, planeType string, planeName string, scope string) ([]string, error) {
	secretsList, err := c.KubernetesSecretClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	items := secretsList.Items
	res := []string{}
	for _, item := range items {
		res = append(res, item.Name)
	}
	return res, nil
}
