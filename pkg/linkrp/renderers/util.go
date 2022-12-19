// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"fmt"
	"strconv"
	"strings"

	conv "github.com/project-radius/radius/pkg/armrpc/api/conv"
	coreDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func ValidateApplicationID(application string) (resources.ID, error) {
	app := &coreDatamodel.Application{}
	if application != "" {
		appId, err := resources.ParseResource(application)
		if err != nil {
			return resources.ID{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("failed to parse application from the property: %s", err.Error()))
		}
		if !strings.EqualFold(appId.Type(), app.ResourceTypeName()) {
			return resources.ID{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("provided application id type %q is not a valid type.", appId.Type()))
		}
		return appId, nil
	}
	return resources.ID{}, nil
}

func MustParseInt32(inp interface{}) (int32, error) {
	if inp != nil {
		switch val := inp.(type) {
		case float64:
			return int32(val), nil
		case int32:
			return val, nil
		case string:
			converted, _ := strconv.Atoi(val)
			return int32(converted), nil
		default:
			return 0, fmt.Errorf("unhandled type for the input %s", val)
		}
	}
	return 0, errors.New("input must not be nil")
}
