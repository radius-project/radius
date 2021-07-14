// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"

	"github.com/Azure/radius/pkg/rad/armtemplate"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

type KubernetesDeploymentClient struct {
	Client    dynamic.Interface
	Namespace string
}

func (c KubernetesDeploymentClient) Deploy(ctx context.Context, content string) error {
	template, err := armtemplate.Parse(content)
	if err != nil {
		return err
	}

	resources, err := armtemplate.Eval(template, armtemplate.TemplateOptions{})
	if err != nil {
		return err
	}

	for _, resource := range resources {
		k8sInfo, err := armtemplate.ConvertToK8s(resource, c.Namespace)
		if err != nil {
			return err
		}

		data, err := k8sInfo.Unstructured.MarshalJSON()
		if err != nil {
			return err
		}

		_, err = c.Client.Resource(k8sInfo.GVR).Namespace(c.Namespace).Patch(ctx, k8sInfo.Name, types.ApplyPatchType, data, v1.PatchOptions{FieldManager: "rad"})
		if err != nil {
			return err
		}
	}

	return nil
}
