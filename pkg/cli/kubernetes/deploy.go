// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/cli/clients"
	"github.com/Azure/radius/pkg/kubernetes"
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const (
	ConditionReady = "Ready"
	TimeoutSeconds = int64(3600) // 1 hour
)

type KubernetesDeploymentClient struct {
	Client    client.Client
	Dynamic   dynamic.Interface
	Typed     *k8s.Clientset
	Namespace string
}

var (
	Scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(Scheme)
	_ = radiusv1alpha3.AddToScheme(Scheme)
	_ = bicepv1alpha3.AddToScheme(Scheme)
	_ = gatewayv1alpha1.AddToScheme(Scheme)
}

func (c KubernetesDeploymentClient) Deploy(ctx context.Context, options clients.DeploymentOptions) (clients.DeploymentResult, error) {
	kind := "DeploymentTemplate"

	// Unmarhsal the content into a deployment template
	// rather than a string.
	armJson := armtemplate.DeploymentTemplate{}

	err := json.Unmarshal([]byte(options.Template), &armJson)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	data, err := json.Marshal(armJson)
	if err != nil {
		return clients.DeploymentResult{}, err
	}

	parameterData, err := json.Marshal(options.Parameters)
	if err != nil {
		return clients.DeploymentResult{}, err
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

	if err != nil {
		return clients.DeploymentResult{}, err
	}

	return c.waitForDeploymentCompletion(ctx, kind, deployment)
}

func (c KubernetesDeploymentClient) waitForDeploymentCompletion(ctx context.Context, kind string, deployment bicepv1alpha3.DeploymentTemplate) (clients.DeploymentResult, error) {

	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(c.Typed.DiscoveryClient))
	mapping, err := restMapper.RESTMapping(schema.GroupKind{Group: bicepv1alpha3.GroupVersion.Group, Kind: kind}, bicepv1alpha3.GroupVersion.Version)
	if err != nil {
		return clients.DeploymentResult{}, err
	}
	watcher, err := c.Dynamic.Resource(mapping.Resource).Namespace(deployment.Namespace).Watch(ctx,
		v1.ListOptions{
			Watch:          true,
			FieldSelector:  fmt.Sprintf("metadata.name==%s,metadata.namespace==%s", deployment.Name, deployment.Namespace),
			TimeoutSeconds: int64Ptr(TimeoutSeconds),
		})

	if err != nil {
		return clients.DeploymentResult{}, err
	}

	defer watcher.Stop()

	for {
		select {
		case event := <-watcher.ResultChan():
			crd, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}

			// This is a double check, should be filtered already from field selector
			if crd.GetName() != deployment.Name || crd.GetNamespace() != deployment.Namespace {
				continue
			}

			deploymentTemplate := bicepv1alpha3.DeploymentTemplate{}

			err = runtime.DefaultUnstructuredConverter.FromUnstructured(crd.Object, &deploymentTemplate)
			if err != nil {
				continue
			}

			if event.Type == watch.Added || event.Type == watch.Modified {
				templateCondition := meta.FindStatusCondition(deploymentTemplate.Status.Conditions, ConditionReady)
				if templateCondition != nil && templateCondition.Status == v1.ConditionTrue {
					// Done with deployment
					return clients.DeploymentResult{}, nil
				} else if templateCondition != nil && templateCondition.Status == v1.ConditionFalse {
					return clients.DeploymentResult{}, fmt.Errorf("Deployment failed: %s", templateCondition.Message)
				}
			}
		case <-ctx.Done():
			return clients.DeploymentResult{}, err
		}
	}
}

func int64Ptr(i int64) *int64 {
	return &i
}
