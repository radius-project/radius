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
	"strings"

	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ValidateRequest validates the resource in the incoming request.
func ValidateRequest(ctx context.Context, newResource *datamodel.SecretStore, oldResource *datamodel.SecretStore, options *controller.Options) (rest.Response, error) {
	if newResource.Properties.Type != datamodel.SecretTypeCert {
		return rest.NewBadRequestResponse(fmt.Sprintf("secret store type %s is not supported.", newResource.Properties.Type)), nil
	}

	if oldResource != nil {
		if oldResource.Properties.Type != newResource.Properties.Type {
			return rest.NewBadRequestResponse("type cannot be changed."), nil
		}

		if newResource.Properties.Resource == "" {
			newResource.Properties.Resource = oldResource.Properties.Resource
		} else if oldResource.Properties.Resource != newResource.Properties.Resource {
			return rest.NewBadRequestResponse(fmt.Sprintf("'%s' of $.properties.resource must be same as '%s'.", newResource.Properties.Resource, oldResource.Properties.Resource)), nil
		}
	}

	refResourceID := newResource.Properties.Resource
	if refResourceID == "" {
		// In this case, Radius creates and manages new Kubernetes secret.
		for k, secret := range newResource.Properties.Data {
			if secret.ValueFrom != nil {
				return rest.NewBadRequestResponse(fmt.Sprintf("data[%s] must not set valueFrom.", k)), nil
			}
		}
	} else {
		if _, _, err := fromResourceID(refResourceID); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func getNamespace(ctx context.Context, res *datamodel.SecretStore, options *controller.Options) (string, error) {
	prop := res.Properties
	if prop.Application != "" {
		app, err := store.GetResource[datamodel.Application](ctx, options.StorageClient, prop.Application)
		if err != nil {
			return "", err
		}
		compute := app.Properties.Status.Compute
		if compute != nil && compute.KubernetesCompute.Namespace != "" {
			return compute.KubernetesCompute.Namespace, nil
		}
	}

	if prop.Environment != "" {
		env, err := store.GetResource[datamodel.Environment](ctx, options.StorageClient, prop.Environment)
		if err != nil {
			return "", err
		}
		namespace := env.Properties.Compute.KubernetesCompute.Namespace
		if namespace != "" {
			return namespace, nil
		}
	}

	return "", errors.New("no Kubernetes namespace")
}

func toResourceID(ns, name string) string {
	if ns == "" {
		return name
	}
	return fmt.Sprintf("%s/%s", ns, name)
}

func fromResourceID(id string) (ns string, name string, err error) {
	res := strings.Split(id, "/")
	if len(res) == 2 {
		ns, name = res[0], res[1]
	} else if len(res) == 1 {
		ns, name = "", res[0]
	} else {
		err = fmt.Errorf("'%s' is the invalid resource id", id)
		return
	}

	if name != "" && !kubernetes.IsValidObjectName(name) {
		err = fmt.Errorf("'%s' is the invalid resource name. This must be at most 63 alphanumeric characters or '-'", name)
		return
	}

	if ns != "" && !kubernetes.IsValidObjectName(ns) {
		err = fmt.Errorf("'%s' is the invalid namespace. This must be at most 63 alphanumeric characters or '-'", ns)
		return
	}

	return
}

// UpsertSecret upserts secret store data to backing secret store.
func UpsertSecret(ctx context.Context, newResource, old *datamodel.SecretStore, options *controller.Options) (rest.Response, error) {
	ref := newResource.Properties.Resource
	if ref == "" && old != nil {
		ref = old.Properties.Resource
	}

	ns, name, err := fromResourceID(ref)
	if err != nil {
		return nil, err
	}

	if ns == "" {
		if ns, err = getNamespace(ctx, newResource, options); err != nil {
			return nil, err
		}
	}

	if name == "" {
		name = newResource.Name
	}

	newResource.Properties.Resource = toResourceID(ns, name)

	ksecret := &corev1.Secret{}
	err = options.KubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: name}, ksecret)
	if apierrors.IsNotFound(err) {
		app, _ := resources.ParseResource(newResource.Properties.Application)
		ksecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    kubernetes.MakeDescriptiveLabels(app.Name(), name, ResourceTypeName),
			},
			Data: map[string][]byte{},
		}
	} else if err != nil {
		return nil, err
	}

	for k, secret := range newResource.Properties.Data {
		val := to.String(secret.Value)
		if val != "" {
			if newResource.Properties.Type == datamodel.SecretTypeCert || secret.Encoding == datamodel.SecretValueEncodingBase64 {
				ksecret.Data[k] = []byte(val)
			} else {
				base64.StdEncoding.Encode(ksecret.Data[k], []byte(val))
			}

			// Remove secret from metadata.
			secret.Value = nil
		} else {
			if secret.ValueFrom != nil {
				key := secret.ValueFrom.Name
				if k != key {
					return rest.NewBadRequestResponse(fmt.Sprintf("%s key name must be same as valueFrom.name %s.", k, key)), nil
				}
			}

			_, ok := ksecret.Data[k]
			if !ok {
				return rest.NewBadRequestResponse(fmt.Sprintf("%s does not have key, %s.", newResource.Properties.Resource, k)), nil
			}
		}
	}

	switch newResource.Properties.Type {
	case datamodel.SecretTypeCert:
		ksecret.Type = corev1.SecretTypeTLS
	case datamodel.SecretTypeGeneric:
		ksecret.Type = corev1.SecretTypeOpaque
	default:
		return rest.NewBadRequestResponse(fmt.Sprintf("%s is invalid secret type.", newResource.Properties.Type)), nil
	}

	if ksecret.ResourceVersion == "" {
		err = options.KubeClient.Create(ctx, ksecret)
	} else {
		err = options.KubeClient.Update(ctx, ksecret)
	}

	if err != nil {
		return nil, err
	}

	newResource.Properties.Status.OutputResources = []rpv1.OutputResource{
		{
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &resourcemodel.ResourceType{
					Type:     resourcekinds.Secret,
					Provider: resourcemodel.ProviderKubernetes,
				},
				Data: resourcemodel.KubernetesIdentity{
					Kind:       resourcekinds.Secret,
					APIVersion: "v1",
					Name:       name,
					Namespace:  ns,
				},
			},
		},
	}

	return nil, nil
}

// DeleteRadiusSecret deletes a secret if the secret is managed by Radius.
func DeleteRadiusSecret(ctx context.Context, oldResource *datamodel.SecretStore, options *controller.Options) (rest.Response, error) {
	or := oldResource.Properties.Status.OutputResources
	ki := resourcemodel.KubernetesIdentity{}
	if len(or) > 0 {
		if err := store.DecodeMap(or[0].Identity.Data, &ki); err != nil {
			return nil, nil
		}
	}

	if ki.Kind == resourcekinds.Secret {
		ksecret := &corev1.Secret{}
		err := options.KubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: ki.Namespace, Name: ki.Name}, ksecret)
		if !apierrors.IsNotFound(err) && err != nil {
			return nil, err
		}

		// Delete only Radius managed resource.
		if _, ok := ksecret.Labels[kubernetes.LabelRadiusResourceType]; ok {
			if err := options.KubeClient.Delete(ctx, ksecret); err != nil {
				return nil, err
			}
		}
	}

	return nil, nil
}
