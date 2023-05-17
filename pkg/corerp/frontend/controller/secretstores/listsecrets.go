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
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/to"
)

const (
	OperationListSecrets = "LISTSECRETS"
)

// ListSecrets is the controller implementing listSecret custom action for Applications.Core/secretStores.
type ListSecrets struct {
	ctrl.Operation[*datamodel.SecretStore, datamodel.SecretStore]
}

// NewListSecrets creates a new ListSecrets controller.
func NewListSecrets(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListSecrets{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.SecretStore]{
				RequestConverter:  converter.SecretStoreModelFromVersioned,
				ResponseConverter: converter.SecretStoreModelToVersioned,
			},
		),
	}, nil
}

// Run reads secret store metadata and returns the secret data from kubernetes secret. Currently, we support only kubernetes secret store.
func (l *ListSecrets) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	resource, _, err := l.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if resource == nil {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	ksecret, err := getSecretFromOutputResources(resource.Properties.Status.OutputResources, l.Options())
	if err != nil {
		return nil, fmt.Errorf("failed to get secret from output resource: %w", err)
	}

	if ksecret == nil {
		return nil, errors.New("referenced secret is not found")
	}

	resp := &datamodel.SecretStoreListSecrets{
		Type: resource.Properties.Type,
		Data: map[string]*datamodel.SecretStoreDataValue{},
	}

	for k, d := range resource.Properties.Data {
		key := k
		if d.ValueFrom != nil {
			key = d.ValueFrom.Name
		}

		val, ok := ksecret.Data[key]
		if !ok {
			return nil, fmt.Errorf("cannot find %s key from secret data", key)
		}

		// Kubernetes secret data is always base64-encoded. If the encoding is raw, we need to decode it.
		if d.Encoding == datamodel.SecretValueEncodingRaw {
			val, err = base64.StdEncoding.DecodeString(string(val))
			if err != nil {
				return nil, fmt.Errorf("%s is the invalid base64 encoded value: %w", key, err)
			}
		}

		resp.Data[k] = &datamodel.SecretStoreDataValue{
			Encoding: d.Encoding,
			Value:    to.Ptr(string(val)),
		}
	}

	return rest.NewOKResponse(resp), nil
}
