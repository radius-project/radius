// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"encoding/json"

	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/kubernetes"
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type KubernetesDeploymentClient struct {
	Client    client.Client
	Namespace string
}

func (c KubernetesDeploymentClient) Deploy(ctx context.Context, content string, parameters clients.DeploymentParameters) error {
	kind := "DeploymentTemplate"

	// Unmarhsal the content into a deployment template
	// rather than a string.
	armJson := armtemplate.DeploymentTemplate{}

	err := json.Unmarshal([]byte(content), &armJson)
	if err != nil {
		return err
	}

	data, err := json.Marshal(armJson)
	if err != nil {
		return err
	}

	parameterData, err := json.Marshal(parameters)
	if err != nil {
		return err
	}

	deployment := bicepv1alpha3.DeploymentTemplate{
		TypeMeta: v1.TypeMeta{
			APIVersion: "bicep.dev/v1alpha3",
			Kind:       kind,
		},
		ObjectMeta: v1.ObjectMeta{
			GenerateName: "deploymenttemplate-",
			Namespace:    c.Namespace,
		},
		Spec: bicepv1alpha3.DeploymentTemplateSpec{
			Content:    &runtime.RawExtension{Raw: data},
			Parameters: &runtime.RawExtension{Raw: parameterData},
		},
	}

	err = c.Client.Create(ctx, &deployment, &client.CreateOptions{FieldManager: kubernetes.FieldManager})
	return err
}
