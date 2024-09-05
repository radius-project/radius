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

package pubsubbrokers

import (
	"context"

	"github.com/radius-project/radius/pkg/daprrp/datamodel"
	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/portableresources/handlers"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/portableresources/renderers/dapr"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

type Processor struct {
	Client runtime_client.Client
}

// Process validates resource properties, and applies output values from the recipe output. If the resource is
// being provisioned manually, it creates a Dapr component in Kubernetes.
func (p *Processor) Process(ctx context.Context, resource *datamodel.DaprPubSubBroker, options processors.Options) error {
	validator := processors.NewValidator(&resource.ComputedValues, &resource.SecretValues, &resource.Properties.Status.OutputResources, resource.Properties.Status.Recipe)
	validator.AddResourcesField(&resource.Properties.Resources)
	validator.AddComputedStringField("componentName", &resource.Properties.ComponentName, func() (string, *processors.ValidationError) {
		return kubernetes.NormalizeDaprResourceName(resource.Name), nil
	})

	err := validator.SetAndValidate(options.RecipeOutput)
	if err != nil {
		return err
	}

	if resource.Properties.ResourceProvisioning != portableresources.ResourceProvisioningManual {
		// If the resource is being provisioned by recipe then we expect the recipe to create the Dapr Component
		// in Kubernetes. At this point we're done so we can just return.
		return nil
	}

	// If the resource is being provisioned manually then *we* are responsible for creating the Dapr Component.
	// Let's do this now.

	// DaprPubSubBroker resources may or may not be application scoped.
	// Some Dapr Components can be specific to a single application, they would be application scoped and have
	// resource.Properties.Application populated, while others could be shared across multiple applications and
	// would not have resource.Properties.Application populated.
	var applicationID resources.ID
	if resource.Properties.Application != "" {
		applicationID, err = resources.ParseResource(resource.Properties.Application)
		if err != nil {
			return err // This should already be validated by this point.
		}
	}

	component, err := dapr.ConstructDaprGeneric(
		dapr.DaprGeneric{
			Auth:     resource.Properties.Auth,
			Metadata: resource.Properties.Metadata,
			Type:     to.Ptr(resource.Properties.Type),
			Version:  to.Ptr(resource.Properties.Version),
		},
		options.RuntimeConfiguration.Kubernetes.Namespace,
		resource.Properties.ComponentName,
		applicationID.Name(),
		resource.Name,
		dapr_ctrl.DaprPubSubBrokersResourceType)
	if err != nil {
		return err
	}

	err = kubeutil.PatchNamespace(ctx, p.Client, component.GetNamespace())
	if err != nil {
		return &processors.ResourceError{Inner: err}
	}

	err = handlers.CheckDaprResourceNameUniqueness(ctx, p.Client, resource.Properties.ComponentName, options.RuntimeConfiguration.Kubernetes.Namespace, resource.Name, dapr_ctrl.DaprPubSubBrokersResourceType)
	if err != nil {
		return &processors.ValidationError{Message: err.Error()}
	}

	err = p.Client.Patch(ctx, &component, runtime_client.Apply, &runtime_client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return &processors.ResourceError{Inner: err}
	}

	deployed := rpv1.NewKubernetesOutputResource("Component", &component, metav1.ObjectMeta{Name: component.GetName(), Namespace: component.GetNamespace()})
	deployed.RadiusManaged = to.Ptr(true)
	resource.Properties.Status.OutputResources = append(resource.Properties.Status.OutputResources, deployed)

	return nil
}

// Delete implements the processors.Processor interface for DaprPubSubBroker resources. If the resource is being
// provisioned manually, it deletes the Dapr component in Kubernetes.
func (p *Processor) Delete(ctx context.Context, resource *datamodel.DaprPubSubBroker, options processors.Options) error {
	if resource.Properties.ResourceProvisioning != portableresources.ResourceProvisioningManual {
		// If the resource was provisioned by recipe then we expect the recipe engine to delete the Dapr Component
		// in Kubernetes. At this point we're done so we can just return.
		return nil
	}

	// DaprPubSubBroker resources may or may not be application scoped.
	// Some Dapr Components can be specific to a single application, they would be application scoped and have
	// resource.Properties.Application populated, while others could be shared across multiple applications and
	// would not have resource.Properties.Application populated.
	var err error
	var applicationID resources.ID
	if resource.Properties.Application != "" {
		applicationID, err = resources.ParseResource(resource.Properties.Application)
		if err != nil {
			return err
		}
	}

	component := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": dapr.DaprAPIVersion,
			"kind":       dapr.DaprKind,
			"metadata": map[string]any{
				"namespace": options.RuntimeConfiguration.Kubernetes.Namespace,
				"name":      kubernetes.NormalizeDaprResourceName(resource.Properties.ComponentName),
				"labels":    kubernetes.MakeDescriptiveDaprLabels(applicationID.Name(), resource.Name, dapr_ctrl.DaprPubSubBrokersResourceType),
			},
		},
	}

	err = p.Client.Delete(ctx, &component)
	if err != nil {
		return &processors.ResourceError{Inner: err}
	}

	return nil
}
