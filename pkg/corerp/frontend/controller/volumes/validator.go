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

package volumes

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	secretProviderClassesCRD = "secretproviderclasses.secrets-store.csi.x-k8s.io"
)

// # Function Explanation
//
// ValidateRequest checks if the new resource kind is valid and if a Secret CSI driver is installed for the resource kind.
// If not, it returns a BadRequestResponse.
func ValidateRequest(ctx context.Context, newResource *datamodel.VolumeResource, oldResource *datamodel.VolumeResource, options *controller.Options) (rest.Response, error) {
	csiCRDValidationRequired := false

	switch newResource.Properties.Kind {
	case datamodel.AzureKeyVaultVolume:
		csiCRDValidationRequired = true
	default:
		return rest.NewBadRequestResponse(fmt.Sprintf("invalid resource kind: %s", newResource.Properties.Kind)), nil
	}

	// TODO: Validate if Secret CSI driver required for ".Properties.Kind" is installed.

	if csiCRDValidationRequired {
		crd := &apiextv1.CustomResourceDefinition{}
		err := options.KubeClient.Get(ctx, client.ObjectKey{Name: secretProviderClassesCRD}, crd)
		if apierrors.IsNotFound(err) {
			return rest.NewBadRequestResponse("Your volume requires secret store CSI driver. Please install it by following https://secrets-store-csi-driver.sigs.k8s.io/."), nil
		} else if err != nil {
			return nil, err
		}
	}

	return nil, nil
}
