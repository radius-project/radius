// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/cli/armtemplate"
	"gopkg.in/yaml.v2"
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

		d, err := yaml.Marshal(k8sInfo.Unstructured)
		fmt.Println(string(d))
		_, err = c.Client.Resource(k8sInfo.GVR).Namespace(c.Namespace).Patch(ctx, k8sInfo.Name, types.ApplyPatchType, data, v1.PatchOptions{FieldManager: "rad"})
		if err != nil {
			return err
		}
	}

	return nil
}
