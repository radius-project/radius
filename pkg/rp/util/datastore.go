// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package util

import (
	"context"
	"errors"
	"fmt"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	resources "github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

// FetchScopeResource fetches environment or application resource linked to resource.
func FetchScopeResource(ctx context.Context, sp dataprovider.DataStorageProvider, scopeID string, resource v1.DataModelInterface) error {
	id, err := resources.ParseResource(scopeID)
	if err != nil {
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("%s is not a valid resource id for %s.", scopeID, resource.ResourceTypeName()))
	}

	if !strings.EqualFold(id.Type(), resource.ResourceTypeName()) {
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("linked %q has invalid %s resource type.", scopeID, resource.ResourceTypeName()))
	}
	sc, err := sp.GetStorageClient(ctx, id.Type())
	if err != nil {
		return err
	}

	res, err := sc.Get(ctx, id.String())
	if errors.Is(&store.ErrNotFound{}, err) {
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("linked resource %s does not exist", scopeID))
	}
	if err != nil {
		return fmt.Errorf("failed to fetch %s. Error: %w", scopeID, err)
	}

	err = res.As(resource)
	if err != nil {
		return err
	}

	return nil
}
