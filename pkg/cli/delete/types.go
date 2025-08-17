/*
Copyright 2025 The Radius Authors.

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

package delete

import (
	"context"

	"github.com/radius-project/radius/pkg/cli/clients"
)

//go:generate mockgen -typed -destination=./mock_delete.go -package=delete -self_package github.com/radius-project/radius/pkg/cli/delete github.com/radius-project/radius/pkg/cli/delete Interface

// Interface is the interface for executing delete operations in the CLI.
type Interface interface {
	// DeleteApplicationWithProgress deletes an application with progress reporting. This is used to
	// provide feedback to the user during the deletion process. It returns a boolean indicating
	DeleteApplicationWithProgress(ctx context.Context, client clients.ApplicationsManagementClient, options clients.DeleteOptions) (bool, error)
}

type Impl struct {
}

func (i *Impl) DeleteApplicationWithProgress(ctx context.Context, client clients.ApplicationsManagementClient, options clients.DeleteOptions) (bool, error) {
	return DeleteApplicationWithProgress(ctx, client, options)
}
