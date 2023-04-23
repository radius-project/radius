// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretstores

import (
	"context"
	"fmt"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/frontend/controller/util"
	"github.com/project-radius/radius/pkg/kubernetes"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*CreateOrUpdateSecretStore)(nil)

const (
	envNamespaceQuery = "properties.compute.kubernetes.namespace"
	appNamespaceQuery = "properties.status.compute.kubernetes.namespace"
)

// CreateOrUpdateSecretStore is the controller implementation to create or update application resource.
type CreateOrUpdateSecretStore struct {
	ctrl.Operation[*datamodel.SecretStore, datamodel.SecretStore]
	KubeClient runtimeclient.Client
}

// NewCreateOrUpdateSecretStore creates a new instance of CreateOrUpdateSecretStore.
func NewCreateOrUpdateSecretStore(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateSecretStore{
		Operation: ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.SecretStore]{
				RequestConverter:  converter.SecretStoreModelFromVersioned,
				ResponseConverter: converter.SecretStoreModelToVersioned,
			}),
		KubeClient: opts.KubeClient,
	}, nil
}

func (a *CreateOrUpdateSecretStore) populateKubernetesNamespace(ctx context.Context, newResource, old *datamodel.SecretStore) (rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	kubeNamespace := ""

	// Check if another environment resource is using namespace
	envID, err := resources.ParseResource(newResource.Properties.Environment)
	if err != nil {
		return rest.NewBadRequestResponse(fmt.Sprintf("Environment %s for application %s could not be found", envID.Name(), serviceCtx.ResourceID.Name())), nil
	}

	result, err := util.FindResources(ctx, envID.RootScope(), envID.Type(), envNamespaceQuery, kubeNamespace, a.StorageClient())
	if err != nil {
		return nil, err
	}
	if len(result.Items) > 0 {
		return rest.NewConflictResponse(fmt.Sprintf("Environment %s with the same namespace (%s) already exists", envID.Name(), kubeNamespace)), nil
	}

	// Check if another application resource is using namespace
	result, err = util.FindResources(ctx, serviceCtx.ResourceID.RootScope(), serviceCtx.ResourceID.Type(), appNamespaceQuery, kubeNamespace, a.StorageClient())
	if err != nil {
		return nil, err
	}
	if len(result.Items) > 0 {
		app := &datamodel.SecretStore{}
		if err := result.Items[0].As(app); err != nil {
			return nil, err
		}

		// If a different resource has the same namespace, return a conflict
		// Otherwise, continue and update the resource
		if old == nil || app.ID != old.ID {
			return rest.NewConflictResponse(fmt.Sprintf("Application %s with the same namespace (%s) already exists", app.ID, kubeNamespace)), nil
		}
	}

	if !kubernetes.IsValidObjectName(kubeNamespace) {
		return rest.NewBadRequestResponse(fmt.Sprintf("'%s' is the invalid namespace. This must be at most 63 alphanumeric characters or '-'. Please specify a valid namespace using 'kubernetesNamespace' extension in '$.properties.extensions[*]'.", kubeNamespace)), nil
	}

	if old != nil {
		c := old.Properties.Status.Compute
		if c != nil && c.Kind == rpv1.KubernetesComputeKind && c.KubernetesCompute.Namespace != kubeNamespace {
			return rest.NewBadRequestResponse(fmt.Sprintf("Updating an application's Kubernetes namespace from '%s' to '%s' requires the application to be deleted and redeployed. Please delete your application and try again.", c.KubernetesCompute.Namespace, kubeNamespace)), nil
		}
	}

	// Populate kubernetes namespace to internal metadata property for query indexing.
	newResource.Properties.Status.Compute = &rpv1.EnvironmentCompute{
		Kind:              rpv1.KubernetesComputeKind,
		KubernetesCompute: rpv1.KubernetesComputeProperties{Namespace: kubeNamespace},
	}

	// TODO: Move it to backend controller - https://github.com/project-radius/radius/issues/4742
	err = a.KubeClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: kubeNamespace}})
	if apierrors.IsAlreadyExists(err) {
		logger.Info("Using existing namespace", "namespace", kubeNamespace)
	} else if err != nil {
		return nil, err
	} else {
		logger.Info("Created the namespace", "namespace", kubeNamespace)
	}

	return nil, nil
}

// Run executes CreateOrUpdateSecretStore operation.
func (a *CreateOrUpdateSecretStore) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := a.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := a.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := a.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := rp_frontend.PrepareRadiusResource(ctx, newResource, old, a.Options()); r != nil || err != nil {
		return r, err
	}

	if r, err := ValidateRequest(ctx, newResource, old, a.Options()); r != nil || err != nil {
		return r, err
	}

	// support only kubernetes.

	// 1. Create Kubernetes secret
	//    Check if kuberentes secret exists or not
	//    If doesn't exist, create secret
	//    If exists, then check it is managed secret or not.
	// 2. Refer Kubernetes secret
	//    Ensure that secret exists
	//

	if r, err := a.populateKubernetesNamespace(ctx, newResource, old); r != nil || err != nil {
		return r, err
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := a.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return a.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
