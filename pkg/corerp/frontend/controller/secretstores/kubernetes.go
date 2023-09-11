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

package secretstores

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/kubernetes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/store"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func getOrDefaultType(t datamodel.SecretType) (datamodel.SecretType, error) {
	var err error
	switch t {
	case datamodel.SecretTypeNone:
		t = datamodel.SecretTypeGeneric
	case datamodel.SecretTypeCert:
	case datamodel.SecretTypeGeneric:
	default:
		err = fmt.Errorf("'%s' is invalid secret type", t)
	}
	return t, err
}

func getOrDefaultEncoding(t datamodel.SecretType, e datamodel.SecretValueEncoding) (datamodel.SecretValueEncoding, error) {
	var err error
	switch e {
	case datamodel.SecretValueEncodingBase64:
		// no-op
	case datamodel.SecretValueEncodingNone:
		// certificate value must be base64-encoded.
		if t == datamodel.SecretTypeCert {
			e = datamodel.SecretValueEncodingBase64
		} else {
			e = datamodel.SecretValueEncodingRaw
		}
	case datamodel.SecretValueEncodingRaw:
		if t == datamodel.SecretTypeCert {
			err = fmt.Errorf("%s type doesn't support %s", datamodel.SecretTypeCert, datamodel.SecretValueEncodingRaw)
		}
	default:
		err = fmt.Errorf("%s is the invalid encoding type", e)
	}

	return e, err
}

// ValidateAndMutateRequest checks the type and encoding of the secret store, and ensures that the secret store data is
// valid. If any of these checks fail, a BadRequestResponse is returned.
func ValidateAndMutateRequest(ctx context.Context, newResource *datamodel.SecretStore, oldResource *datamodel.SecretStore, options *controller.Options) (rest.Response, error) {
	var err error
	newResource.Properties.Type, err = getOrDefaultType(newResource.Properties.Type)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	if oldResource != nil {
		if oldResource.Properties.Type != newResource.Properties.Type {
			return rest.NewBadRequestResponse(fmt.Sprintf("$.properties.type cannot change from '%s' to '%s'.", oldResource.Properties.Type, newResource.Properties.Type)), nil
		}

		if newResource.Properties.Resource == "" {
			newResource.Properties.Resource = oldResource.Properties.Resource
		}
	}

	refResourceID := newResource.Properties.Resource
	if _, _, err := fromResourceID(refResourceID); err != nil {
		return nil, err
	}

	for k, secret := range newResource.Properties.Data {
		// Kubernetes secret does not support valueFrom. Note that this property is reserved to
		// reference the secret in the external secret stores, such as Azure KeyVault and AWS secrets manager.
		if secret.ValueFrom != nil && secret.ValueFrom.Name != "" {
			return rest.NewBadRequestResponse(fmt.Sprintf("$.properties.data[%s].valueFrom.Name is specified. Kubernetes secret resource doesn't support secret reference. ", k)), nil
		}

		secret.Encoding, err = getOrDefaultEncoding(newResource.Properties.Type, secret.Encoding)
		if err != nil {
			return rest.NewBadRequestResponse(fmt.Sprintf("'%s' encoding is not valid: %q", k, err)), nil
		}

		if refResourceID == "" && secret.Value == nil {
			return rest.NewBadRequestResponse(fmt.Sprintf("$.properties.data[%s].Value must be given to create the secret.", k)), nil
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

// UpsertSecret creates or updates a Kubernetes secret based on the incoming request and returns the secret's location in
// the output resource.
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

	if old != nil && old.Properties.Resource != newResource.Properties.Resource {
		return rest.NewBadRequestResponse(fmt.Sprintf("'%s' of $.properties.resource must be same as '%s'.", newResource.Properties.Resource, old.Properties.Resource)), nil
	}

	ksecret := &corev1.Secret{}
	err = options.KubeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: ns, Name: name}, ksecret)
	if apierrors.IsNotFound(err) {
		// If resource in incoming request references resource, then the resource must exist.
		if ref != "" {
			return rest.NewBadRequestResponse(fmt.Sprintf("'%s' referenced resource does not exist.", ref)), nil
		}
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

	updateRequired := false
	for k, secret := range newResource.Properties.Data {
		val := to.String(secret.Value)
		if val != "" {
			// Kubernetes secret data expects base64 encoded value.
			if secret.Encoding == datamodel.SecretValueEncodingRaw {
				encoded := base64.StdEncoding.EncodeToString([]byte(val))
				ksecret.Data[k] = []byte(encoded)
			} else {
				ksecret.Data[k] = []byte(val)
			}
			updateRequired = true
			// Remove secret from metadata before storing it to data store.
			secret.Value = nil
		} else {
			if _, ok := ksecret.Data[k]; !ok {
				return rest.NewBadRequestResponse(fmt.Sprintf("'%s' resource does not have key, '%s'.", newResource.Properties.Resource, k)), nil
			}
		}
	}

	if ksecret.ResourceVersion == "" {
		switch newResource.Properties.Type {
		case datamodel.SecretTypeCert:
			ksecret.Type = corev1.SecretTypeTLS
		case datamodel.SecretTypeGeneric:
			ksecret.Type = corev1.SecretTypeOpaque
		}
		err = options.KubeClient.Create(ctx, ksecret)
	} else if updateRequired {
		err = options.KubeClient.Update(ctx, ksecret)
	}

	if err != nil {
		return nil, err
	}

	// In order to get the secret data, we need to get the actual secret location from output resource.
	newResource.Properties.Status.OutputResources = []rpv1.OutputResource{
		{
			LocalID: rpv1.LocalIDSecret,
			ID: resources_kubernetes.IDFromParts(
				resources_kubernetes.PlaneNameTODO,
				"",
				resources_kubernetes.KindSecret,
				ns,
				name),
		},
	}

	return nil, nil
}

// DeleteRadiusSecret deletes the Kubernetes secret associated with the given secret store if it is a
// Radius managed resource.
func DeleteRadiusSecret(ctx context.Context, oldResource *datamodel.SecretStore, options *controller.Options) (rest.Response, error) {
	ksecret, err := getSecretFromOutputResources(oldResource.Properties.Status.OutputResources, options)
	if err != nil {
		return nil, err
	}

	if ksecret != nil {
		// Delete only Radius managed resource.
		if _, ok := ksecret.Labels[kubernetes.LabelRadiusResourceType]; ok {
			if err := options.KubeClient.Delete(ctx, ksecret); err != nil {
				return nil, err
			}
		}
	}

	return nil, nil
}

func getSecretFromOutputResources(resources []rpv1.OutputResource, options *controller.Options) (*corev1.Secret, error) {
	name, ns := "", ""
	for _, resource := range resources {
		if strings.EqualFold(resource.ID.Type(), "core/Secret") {
			_, _, ns, name = resources_kubernetes.ToParts(resource.ID)
			break
		}
	}

	ksecret := &corev1.Secret{}
	err := options.KubeClient.Get(context.Background(), runtimeclient.ObjectKey{Namespace: ns, Name: name}, ksecret)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	return ksecret, nil
}
