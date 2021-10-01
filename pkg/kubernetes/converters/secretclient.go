// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package converters

import (
	"context"
	"fmt"

	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/renderers"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SecretClient can be used to look up the value of a SecretValueReference from the underlying Kubernetes Secret that stores it.
type SecretClient struct {
	Client client.Client
}

func (sc *SecretClient) LookupSecretValue(ctx context.Context, status radiusv1alpha3.ResourceStatus, secretReference renderers.SecretValueReference) (string, error) {
	// Each value needs to be looked up in a secret where it's stored. The reference
	// to the secret will be in the output resources.
	outputResource, ok := status.Resources[secretReference.LocalID]
	if !ok {
		return "", fmt.Errorf("could not find a matching resource for LocalID %q", secretReference.LocalID)
	}

	secret := corev1.Secret{}
	err := sc.Client.Get(ctx, client.ObjectKey{Namespace: outputResource.Namespace, Name: outputResource.Name}, &secret)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve secret of dependency: %w", err)
	}

	value, ok := secret.Data[secretReference.ValueSelector]
	if !ok {
		return "", fmt.Errorf("secret did contain expected key: %q", secretReference.ValueSelector)
	}

	return string(value), nil
}
