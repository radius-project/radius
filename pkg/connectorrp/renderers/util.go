// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"fmt"
	"strings"

	coreDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func (r *ErrClientRenderer) Error() string {
	return fmt.Sprintf("code %v: err %v", r.Code, r.Message)
}

func NewClientErrInvalidRequest(message string) *ErrClientRenderer {
	err := new(ErrClientRenderer)
	err.Message = message
	err.Code = armerrors.Invalid
	return err
}

func ValidateApplicationID(application string) (resources.ID, error) {
	app := &coreDatamodel.Application{}
	if application != "" {
		appId, err := resources.Parse(application)
		if err != nil {
			return resources.ID{}, NewClientErrInvalidRequest(fmt.Sprintf("failed to parse application from the property: %s", err.Error()))
		}
		if !strings.EqualFold(appId.Type(), app.ResourceTypeName()) {
			return resources.ID{}, NewClientErrInvalidRequest(fmt.Sprintf("provided application id type %q is not a valid type.", appId.Type()))
		}
		return appId, nil
	}
	return resources.ID{}, nil
}
