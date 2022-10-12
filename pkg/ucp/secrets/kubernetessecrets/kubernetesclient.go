// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetessecrets

import (
	"context"
	"fmt"

	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/secrets"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreV1Types "k8s.io/client-go/kubernetes/typed/core/v1"
)

var _ secrets.Interface = (*Client)(nil)

type Client struct {
	SecretsClient coreV1Types.SecretInterface
}

func (c *Client) CreateSecrets(ctx context.Context, name string, secrets interface{}) error {
	objMetadata := metav1.ObjectMeta{Name: name}
	// c.SecretsClient.Create(ctx, secret, )
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
	secretsRes, err := c.SecretsClient.Create(ctx, &secretObject, metav1.CreateOptions{})
	fmt.Print(secretsRes)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) DeleteSecrets(ctx context.Context, id string) error {
	c.SecretsClient.Delete(ctx, id, metav1.DeleteOptions{})
	return nil
}

func (c *Client) GetSecrets(ctx context.Context, id string) (string, error) {
	_, err := c.SecretsClient.Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return id, nil
}

func (c *Client) ListSecrets(ctx context.Context, planeType string, planeName string, scope string) ([]string, error) {
	secretsList, err :=c.SecretsClient.List(ctx, metav1.ListOptions{})
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
