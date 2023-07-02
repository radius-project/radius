package modules

import (
	"fmt"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rest"
)

// InvalidPlaneTypeErrorResponse returns a 400 response with error code CodeInvalidPlaneType.
func InvalidPlaneTypeErrorResponse(planeType string, supportedPlaneTypes []string) rest.Response {
	return rest.NewBadRequestARMResponse(v1.ErrorResponse{
		Error: v1.ErrorDetails{
			Code:    v1.CodeInvalidPlaneType,
			Message: fmt.Sprintf("/planes/%s is not supported. Supported PlaneType: %s", planeType, strings.Join(supportedPlaneTypes, ",")),
		},
	})
}
