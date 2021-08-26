// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

type KubernetesDeploymentClient struct {
	Client    dynamic.Interface
	Namespace string
}

func (c KubernetesDeploymentClient) Deploy(ctx context.Context, content string) error {
	gvr := schema.GroupVersionResource{
		Group:    "bicep.dev",
		Version:  "v1alpha1",
		Resource: "deploymenttemplates",
	}

	kind := "DeploymentTemplate"

	// TODO name and annotations
	uns := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gvr.Group + "/" + gvr.Version,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"generateName": "arm-",
				"namespace":    c.Namespace,
			},
			"spec": map[string]interface{}{
				"content": content,
			},
		},
	}

	data, err := uns.MarshalJSON()
	if err != nil {
		return err
	}

	_, err = c.Client.Resource(gvr).Namespace(c.Namespace).Patch(ctx, "arm", types.ApplyPatchType, data, v1.PatchOptions{FieldManager: "rad"})
	return nil
}
