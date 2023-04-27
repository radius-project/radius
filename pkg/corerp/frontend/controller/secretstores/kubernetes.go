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
		return rest.NewBadRequestResponse(fmt.Sprintf("secret store type %s is not supported", newResource.Properties.Type)), nil
	}

	if oldResource != nil {
		if oldResource.Properties.Type != newResource.Properties.Type {
			return rest.NewBadRequestResponse("type cannot be changed"), nil
		}

		if newResource.Properties.Resource == "" {
			newResource.Properties.Resource = oldResource.Properties.Resource
		}
	}

	refResourceID := newResource.Properties.Resource
	if refResourceID == "" {
		// Radius creates new secret.
		for _, secret := range newResource.Properties.Data {
			if secret.ValueFrom != nil {
				return rest.NewBadRequestResponse(fmt.Sprintf("%s must not set valueFrom.", newResource.Properties.Resource)), nil
			}
		}
	} else {
		res := strings.Split(newResource.Properties.Resource, "/")
		if len(res) > 2 {
			return rest.NewBadRequestResponse(fmt.Sprintf("invalid resource id %s", newResource.Properties.Resource)), nil
		}
	}

	return nil, nil
}

func getNamespace(ctx context.Context, newResource *datamodel.SecretStore, options *controller.Options) (string, error) {
	newProp := newResource.Properties
	if newProp.Application != "" {
		app, err := store.GetResource[datamodel.Application](ctx, options.StorageClient, newProp.Application)
		if err != nil {
			return "", err
		}
		compute := app.Properties.Status.Compute
		if compute != nil && compute.KubernetesCompute.Namespace != "" {
			return compute.KubernetesCompute.Namespace, nil
		}
	}

	if newProp.Environment != "" {
		env, err := store.GetResource[datamodel.Environment](ctx, options.StorageClient, newProp.Environment)
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

func parseKubernetesResource(id string) (ns string, name string, err error) {
	res := strings.Split(id, "/")
	if len(res) == 2 {
		ns, name = res[0], res[1]
	} else if len(res) == 1 {
		ns, name = "", res[0]
	} else {
		err = errors.New("invalid resource id")
	}

	if !kubernetes.IsValidObjectName(name) {
		err = errors.New("invalid resource name")
	}
	if !kubernetes.IsValidObjectName(ns) {
		err = errors.New("invalid namespace name")
	}

	return
}

func upsertSecret(ctx context.Context, newResource, old *datamodel.SecretStore, options *controller.Options) (rest.Response, error) {
	if old == nil && newResource.Properties.Resource == "" {
		namespace, err := getNamespace(ctx, newResource, options)
		if err != nil {
			return nil, err
		}
		newResource.Properties.Resource = fmt.Sprintf("%s/%s", namespace, newResource.Name)
	}

	namespace, secertName, err := parseKubernetesResource(newResource.Properties.Resource)
	if err != nil {
		return nil, err
	}

	ksecret := &corev1.Secret{}
	err = options.KubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: namespace, Name: secertName}, ksecret)
	if apierrors.IsNotFound(err) {
		app, _ := resources.ParseResource(newResource.Properties.Application)
		secretName := kubernetes.NormalizeResourceName(newResource.Name)
		ksecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
				Labels:    kubernetes.MakeDescriptiveLabels(app.Name(), secretName, ResourceTypeName),
			},
			Data: map[string][]byte{},
		}
	} else if err != nil {
		return nil, err
	}

	for k, secret := range newResource.Properties.Data {
		val := to.String(secret.Value)
		if val == "" {
			_, err := base64.StdEncoding.DecodeString(val)
			if err == nil {
				ksecret.Data[k] = []byte(val)
				continue
			}
			base64.StdEncoding.Encode(ksecret.Data[k], []byte(val))
		} else if secret.ValueFrom != nil {
			key := secret.ValueFrom.Name
			if !kubernetes.IsValidObjectName(key) {
				return rest.NewBadRequestResponse(fmt.Sprintf("%s includes an invalid object name %s", newResource.Properties.Resource, key)), nil
			}

			if k != key {
				return rest.NewBadRequestResponse(fmt.Sprintf("%s includes an invalid key %s", newResource.Properties.Resource, key)), nil
			}

			_, ok := ksecret.Data[key]
			if !ok {
				return rest.NewBadRequestResponse(fmt.Sprintf("%s does not have key %s", newResource.Properties.Resource, key)), nil
			}
		}
	}

	switch newResource.Properties.Type {
	case datamodel.SecretTypeCert:
		ksecret.Type = corev1.SecretTypeTLS
	case datamodel.SecretTypeGeneric:
		ksecret.Type = corev1.SecretTypeOpaque
	default:
		return rest.NewBadRequestResponse(fmt.Sprintf("invalid secret type %s", newResource.Properties.Type)), nil
	}

	if err := options.KubeClient.Create(ctx, ksecret); err != nil {
		return nil, err
	}

	return nil, nil
}

// DeleteRadiusSecret deletes a secret if the secret is managed by Radius.
func DeleteRadiusSecret(ctx context.Context, newResource *datamodel.SecretStore, oldResource *datamodel.SecretStore, options *controller.Options) (rest.Response, error) {
	namespace, secertName, err := parseKubernetesResource(newResource.Properties.Resource)
	if err != nil {
		return nil, err
	}
	ksecret := &corev1.Secret{}
	err = options.KubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: namespace, Name: secertName}, ksecret)
	if err != nil {
		return nil, err
	}

	// Delete only Radius managed resource.
	if _, ok := ksecret.Labels[kubernetes.LabelRadiusResourceType]; ok {
		if err := options.KubeClient.Delete(ctx, ksecret); err != nil {
			return nil, err
		}
	}

	return nil, nil
}
