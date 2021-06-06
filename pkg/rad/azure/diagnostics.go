// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"

	"github.com/Azure/radius/pkg/rad/clients"
)

type ARMDiagnosticsClient struct {
}

var _ clients.DiagnosticsClient = (*ARMDiagnosticsClient)(nil)

func (dc *ARMDiagnosticsClient) Expose(ctx context.Context, options clients.ExposeOptions) error {
	return nil
}

func (dc *ARMDiagnosticsClient) Logs(ctx context.Context, options clients.LogsOptions) error {
	return nil
}
