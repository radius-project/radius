// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretstores

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/kubernetes"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

func (a *CreateOrUpdateSecretStore) getNamespace(ctx context.Context, newResource *datamodel.SecretStore) (string, error) {
	if newResource.Properties.Application != "" {
		res, err := a.StorageClient().Get(ctx, newResource.Properties.Application)
		if err != nil {
			return "", err
		}
		app := &datamodel.Application{}
		if err := res.As(app); err != nil {
			return "", err
		}
		compute := app.Properties.Status.Compute
		if compute != nil && compute.KubernetesCompute.Namespace != "" {
			return compute.KubernetesCompute.Namespace, nil
		}
	}

	if newResource.Properties.Environment != "" {
		res, err := a.StorageClient().Get(ctx, newResource.Properties.Environment)
		if err != nil {
			return "", err
		}
		env := &datamodel.Environment{}
		if err := res.As(env); err != nil {
			return "", err
		}
		namespace := env.Properties.Compute.KubernetesCompute.Namespace
		if namespace != "" {
			return namespace, nil
		}
	}

	return "", errors.New("no Kubernetes namespace")
}

func (a *CreateOrUpdateSecretStore) referenceExistingSecret(ctx context.Context, newResource, old *datamodel.SecretStore) (rest.Response, error) {
	namespace, err := a.getNamespace(ctx, newResource)
	if err != nil {
		return nil, err
	}

	secretName := newResource.Properties.Resource
	// Kubernetes resource name scenario
	res := strings.Split(newResource.Properties.Resource, "/")
	if len(res) < 2 {
		if len(res) == 2 {
			namespace = res[0]
			secretName = res[1]
		}

		if !kubernetes.IsValidObjectName(namespace) {
			return rest.NewBadRequestResponse(fmt.Sprintf("%s includes an invalid namespace", newResource.Properties.Resource)), nil
		}
		if !kubernetes.IsValidObjectName(secretName) {
			return rest.NewBadRequestResponse(fmt.Sprintf("%s includes an invalid secret name", res[1])), nil
		}

		secret := &corev1.Secret{}
		err := a.KubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: namespace, Name: secretName}, secret)
		if apierrors.IsNotFound(err) {
			return rest.NewBadRequestResponse(fmt.Sprintf("referenced secret %s in namespace %s does not exist", secretName, namespace)), nil
		} else if err != nil {
			return nil, err
		}

		for k, s := range newResource.Properties.Data {
			_, sOK := secret.StringData[k]
			_, bOK := secret.Data[k]
			if !sOK && !bOK {
				return rest.NewBadRequestResponse(fmt.Sprintf("referenced secret %s in namespace %s does not contain key %s", secretName, namespace, k)), nil
			}
			s.ValueFrom.Name = k
		}
		return nil, nil
	}

	return rest.NewBadRequestResponse(fmt.Sprintf("invalid resource reference %s", newResource.Properties.Resource)), nil
}

func (a *CreateOrUpdateSecretStore) createKubernetesSecret(ctx context.Context, newResource, old *datamodel.SecretStore) (rest.Response, error) {
	namespace, err := a.getNamespace(ctx, newResource)
	if err != nil {
		return nil, err
	}

	app, _ := resources.ParseResource(newResource.Properties.Application)
	secretName := kubernetes.NormalizeResourceName(newResource.Name)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(app.Name(), secretName, "Applications.Core/secretStores"),
		},
	}

	newResource.Properties.Resource = fmt.Sprintf("%s/%s", namespace, secretName)
	secretData := map[string][]byte{}

	for k, s := range newResource.Properties.Data {
		if to.String(s.Value) == "" {
			return rest.NewBadRequestResponse(fmt.Sprintf("secret data %s is empty", k)), nil
		}

		_, err := base64.StdEncoding.DecodeString(*s.Value)
		if err == nil {
			secretData[k] = []byte(*s.Value)
			continue
		}
		base64.StdEncoding.Encode(secretData[k], []byte(*s.Value))
	}

	secret.Data = secretData

	switch newResource.Properties.Type {
	case datamodel.SecretTypeCert:
		secret.Type = corev1.SecretTypeTLS
	case datamodel.SecretTypeGeneric:
		secret.Type = corev1.SecretTypeOpaque
	default:
		return rest.NewBadRequestResponse(fmt.Sprintf("invalid secret type %s", newResource.Properties.Type)), nil
	}

	if err := a.KubeClient.Create(ctx, secret); err != nil {
		return nil, err
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

	if r, err := a.referenceExistingSecret(ctx, newResource, old); r != nil || err != nil {
		return r, err
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := a.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return a.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
