// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/cli/armtemplate"
	"github.com/Azure/radius/pkg/cli/armtemplate/providers"
	"github.com/Azure/radius/pkg/kubernetes"
	bicepv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/bicep/v1alpha3"
	radiusv1alpha3 "github.com/Azure/radius/pkg/kubernetes/api/radius/v1alpha3"
	"github.com/Azure/radius/pkg/kubernetes/converters"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// DeploymentTemplateReconciler reconciles a Arm object
type DeploymentTemplateReconciler struct {
	client.Client
	meta.RESTMapper
	DynamicClient dynamic.Interface
	Log           logr.Logger
	Scheme        *runtime.Scheme
}

//+kubebuilder:rbac:groups=bicep.dev,resources=deploymenttemplates,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bicep.dev,resources=deploymenttemplates/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bicep.dev,resources=deploymenttemplates/finalizers,verbs=update

func (r *DeploymentTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("deploymenttemplate", req.NamespacedName)

	arm := &bicepv1alpha3.DeploymentTemplate{}
	err := r.Get(ctx, req.NamespacedName, arm)
	if err != nil {
		return ctrl.Result{}, err
	}

	arm.Status.Operations = nil

	result, err := r.ApplyState(ctx, req, arm)

	_ = r.Status().Update(ctx, arm)
	if err != nil {
		return ctrl.Result{}, err
	}

	return result, err
}

// Parses the arm template and deploys individual resources to the cluster
// TODO: Can we avoid parsing resources multiple times by caching?
func (r *DeploymentTemplateReconciler) ApplyState(ctx context.Context, req ctrl.Request, arm *bicepv1alpha3.DeploymentTemplate) (ctrl.Result, error) {
	template, err := armtemplate.Parse(string(arm.Spec.Content.Raw))
	if err != nil {
		return ctrl.Result{}, err
	}

	parameters := map[string]map[string]interface{}{}
	if arm.Spec.Parameters != nil {
		err = json.Unmarshal(arm.Spec.Parameters.Raw, &parameters)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	options := armtemplate.TemplateOptions{
		SubscriptionID: "kubernetes",
		ResourceGroup: armtemplate.ResourceGroup{
			Name: req.Namespace,
		},
		Parameters: parameters,
	}
	resources, err := armtemplate.Eval(template, options)
	if err != nil {
		return ctrl.Result{}, err
	}

	// All previously deployed resources to be used by other resources
	// to fill in variables ex: ([reference(...)])
	deployed := map[string]map[string]interface{}{}
	evaluator := &armtemplate.DeploymentEvaluator{
		Context:   ctx,
		Template:  template,
		Options:   options,
		Deployed:  deployed,
		Variables: map[string]interface{}{},

		CustomActionCallback: func(id string, apiVersion string, action string, payload interface{}) (interface{}, error) {
			return r.InvokeCustomAction(ctx, req.Namespace, id, apiVersion, action, payload)
		},
		ProviderStore: providers.NewK8sStore(r.Log, r.DynamicClient, r.RESTMapper),
	}

	for name, variable := range template.Variables {
		value, err := evaluator.VisitValue(variable)
		if err != nil {
			return ctrl.Result{}, err
		}

		evaluator.Variables[name] = value
	}

	for i, resource := range resources {
		body, err := evaluator.VisitMap(resource.Body)
		if err != nil {
			return ctrl.Result{}, err
		}

		resource.Body = body

		k8sInfo, err := armtemplate.ConvertToK8s(resource, req.NamespacedName.Namespace)
		if err != nil {
			return ctrl.Result{}, err
		}

		// TODO track progress of operations (count of deployed resources) in Status.
		arm.Status.Operations = append(arm.Status.Operations, bicepv1alpha3.DeploymentTemplateOperation{
			Name:      k8sInfo.GetName(),
			Namespace: k8sInfo.GetNamespace(),
		})

		err = r.Client.Get(ctx, client.ObjectKey{
			Namespace: k8sInfo.GetNamespace(),
			Name:      k8sInfo.GetName(),
		}, k8sInfo)

		if err != nil && client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}

		if apierrors.IsNotFound(err) {
			appName, _, resourceType := resource.GetRadiusResourceParts()

			// If this is not an extension resource, which lives outside an application,
			// make sure the application that contains the resource has been created
			if resourceType != "Application" && resource.Provider == nil {
				application := &radiusv1alpha3.Application{}

				err = r.Client.Get(ctx, client.ObjectKey{
					Namespace: k8sInfo.GetNamespace(),
					Name:      appName,
				}, application)
				if err != nil {
					return ctrl.Result{}, err
				}

				err := controllerutil.SetControllerReference(application, k8sInfo, r.Scheme)
				if err != nil {
					return ctrl.Result{}, err
				}
			}

			err = r.Client.Patch(ctx, k8sInfo, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
			if err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second}, nil
		}

		arm.Status.Operations[i].Provisioned = true
		// TODO could remove this dependecy on radiusv1alpha3
		k8sResource := &radiusv1alpha3.Resource{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(k8sInfo.Object, k8sResource)
		if err != nil {
			return ctrl.Result{}, err
		}

		// Transform from k8s representation to arm representation
		//
		// We need to overlay stateful properties over the original definition.
		//
		// For now we just modify the body in place.
		err = converters.ConvertToARMResource(k8sResource, resource.Body)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to convert to ARM representation: %w", err)
		}

		deployed[resource.ID] = resource.Body

		if k8sResource.Status.Phrase != "Ready" {
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second}, nil
		}
	}
	return reconcile.Result{}, nil
}

func (r *DeploymentTemplateReconciler) InvokeCustomAction(ctx context.Context, namespace string, id string, apiVersion string, action string, payload interface{}) (interface{}, error) {
	if action != "listSecrets" {
		return nil, fmt.Errorf("only %q is supported", "listSecrets")
	}

	// We can ignore ID in this case because it reference to the Radius Custom RP name ('radiusv3')
	// The resource ID we actually want is inside the payload.
	type ListSecretsInput = struct {
		TargetID string `json:"targetID"`
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("failed to read listSecrets payload")
	}

	input := ListSecretsInput{}
	err = json.Unmarshal(b, &input)
	if err != nil {
		return nil, errors.New("failed to read listSecrets payload")
	}

	targetID, err := azresources.Parse(input.TargetID)
	if err != nil {
		return nil, fmt.Errorf("resource id %q is invalid: %w", id, err)
	}

	if len(targetID.Types) != 3 {
		return nil, fmt.Errorf("resource id must refer to a Radius resource, was: %q", id)
	}

	unst := unstructured.Unstructured{}
	unst.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "radius.dev",
		Version: "v1alpha3",
		Kind:    armtemplate.GetKindFromArmType(targetID.Types[2].Type),
	})

	err = r.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: kubernetes.MakeResourceName(targetID.Types[1].Name, targetID.Types[2].Name)}, &unst)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve resource matching id %q: %w", id, err)
	}

	resource := radiusv1alpha3.Resource{}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unst.Object, &resource)
	if err != nil {
		return nil, err
	}

	secretValues, err := converters.GetSecretValues(resource.Status)
	if err != nil {
		return nil, err
	}

	secretClient := converters.SecretClient{Client: r.Client}
	values := map[string]interface{}{}
	for key, reference := range secretValues {
		value, err := secretClient.LookupSecretValue(ctx, resource.Status, reference)
		if err != nil {
			return nil, err
		}

		values[key] = value
	}

	return values, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bicepv1alpha3.DeploymentTemplate{}).
		Complete(r)
}
