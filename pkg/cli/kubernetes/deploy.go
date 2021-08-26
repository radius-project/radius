// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"

	"github.com/Azure/radius/pkg/kubernetes"
	bicepv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesDeploymentClient struct {
	Client    client.Client
	Namespace string
}

func (c KubernetesDeploymentClient) Deploy(ctx context.Context, content string) error {
	kind := "DeploymentTemplate"

	// TODO name and annotations
	deployment := bicepv1alpha1.DeploymentTemplate{
		TypeMeta: v1.TypeMeta{
			APIVersion: "bicep.dev/v1alpha1",
			Kind:       kind,
		},
		ObjectMeta: v1.ObjectMeta{
			GenerateName: "deploymenttemplate-",
			Namespace:    c.Namespace,
		},
		Spec: bicepv1alpha1.DeploymentTemplateSpec{
			Content: content,
		},
	}

	err := c.Client.Patch(ctx, &deployment, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	return err
}
