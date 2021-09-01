// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"encoding/json"

	"github.com/Azure/radius/pkg/kubernetes"
	bicepv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesDeploymentClient struct {
	Client    client.Client
	Namespace string
}

func (c KubernetesDeploymentClient) Deploy(ctx context.Context, content string) error {
	kind := "DeploymentTemplate"

	data, err := json.Marshal(content)
	if err != nil {
		return err
	}

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
			Content: &runtime.RawExtension{Raw: data},
		},
	}

	err = c.Client.Create(ctx, &deployment, &client.CreateOptions{FieldManager: kubernetes.FieldManager})
	return err
}
