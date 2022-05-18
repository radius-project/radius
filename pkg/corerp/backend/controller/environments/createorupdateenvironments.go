// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"

	asyncctrl "github.com/project-radius/radius/pkg/corerp/backend/controller"
)

type CreateOrUpdateEnvironmentAsync struct {
}

func NewCreateOrUpdateEnvironmentAsync() asyncctrl.AsyncControllerInterface {
	return &CreateOrUpdateEnvironmentAsync{}
}

func (c *CreateOrUpdateEnvironmentAsync) Run(ctx context.Context) error {
	return nil
}
