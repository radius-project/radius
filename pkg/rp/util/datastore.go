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

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	resources "github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

// FetchScopeResource fetches environment or application resource linked to resource.
func FetchScopeResource(ctx context.Context, sp dataprovider.DataStorageProvider, scopeID string, resource conv.DataModelInterface) error {
	id, err := resources.ParseResource(scopeID)
	if err != nil {
		return conv.NewClientErrInvalidRequest(fmt.Sprintf("%s is not a valid resource for %s.", scopeID, resource.ResourceTypeName()))
	}

	if !strings.EqualFold(id.Type(), resource.ResourceTypeName()) {
		return conv.NewClientErrInvalidRequest(fmt.Sprintf("linked %q has invalid %s resource type.", scopeID, resource.ResourceTypeName()))
	}
	sc, err := sp.GetStorageClient(ctx, id.Type())
	if err != nil {
		return err
	}

	res, err := sc.Get(ctx, id.String())
	if errors.Is(&store.ErrNotFound{}, err) {
		return conv.NewClientErrInvalidRequest(fmt.Sprintf("linked %q does not exist", scopeID))
	}

	const errMsg = "failed to fetch %q for the resource %q. Error: %w"
	if err != nil {
		return fmt.Errorf(errMsg, scopeID, resource, err)
	}

	err = res.As(resource)
	if err != nil {
		return fmt.Errorf(errMsg, scopeID, resource, err)
	}

	return nil
}
